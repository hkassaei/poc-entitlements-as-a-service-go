import Fastify from 'fastify';
import { Type } from '@sinclair/typebox';
import { TypeBoxTypeProvider } from '@fastify/type-provider-typebox';
import { eq } from 'drizzle-orm';
import { config } from './config.js';
import { db, pool, subscribers } from './db.js';
import { createKeyManager } from './kms.js';
import { decryptWithDek, zeroBuffer } from './kms.js';
import { generateVectors, validateAuts } from './milenage.js';

const keyManager = createKeyManager(config);

const app = Fastify({
  logger: {
    level: config.nodeEnv === 'test' ? 'silent' : 'info',
  },
}).withTypeProvider<TypeBoxTypeProvider>();

// Health check
app.get('/health', async () => {
  return { status: 'ok' };
});

// POST /vectors — generate authentication vectors for a subscriber
const VectorsRequest = Type.Object({
  imsi: Type.String(),
  sqn: Type.Optional(Type.Number()),
});

const VectorsResponse = Type.Object({
  rand: Type.String(),
  autn: Type.String(),
  xres: Type.String(),
  ck: Type.String(),
  ik: Type.String(),
});

const ErrorResponse = Type.Object({
  error: Type.String(),
});

// POST /resync — resynchronize SQN after SYNC_FAILURE
const ResyncRequest = Type.Object({
  imsi: Type.String(),
  rand: Type.String(), // base64, RAND from failed challenge
  auts: Type.String(), // base64, 14 bytes from AT_AUTS
});

const ResyncResponse = Type.Object({
  rand: Type.String(),
  autn: Type.String(),
  xres: Type.String(),
  ck: Type.String(),
  ik: Type.String(),
});

app.post(
  '/vectors',
  {
    schema: {
      body: VectorsRequest,
      response: {
        200: VectorsResponse,
        404: ErrorResponse,
      },
    },
  },
  async (request, reply) => {
    const { imsi, sqn: sqnOverride } = request.body;

    // Look up subscriber
    const rows = await db
      .select()
      .from(subscribers)
      .where(eq(subscribers.imsi, imsi))
      .limit(1);

    const subscriber = rows[0];
    if (!subscriber) {
      return reply.status(404).send({ error: 'Subscriber not found' });
    }

    // Unwrap DEK and decrypt Ki/OP
    const dek = await keyManager.unwrapDek(subscriber.kiDekWrapped);
    const ki = decryptWithDek(dek, subscriber.kiEncrypted);
    const op = decryptWithDek(dek, subscriber.opEncrypted);

    // Build SQN (6 bytes, big-endian)
    const sqnValue = sqnOverride ?? subscriber.sqn ?? 0;
    const sqnBuf = Buffer.alloc(6);
    // Write as 48-bit big-endian
    sqnBuf.writeUIntBE(sqnValue, 0, 6);

    // AMF: 0x8000 (separation bit set for LTE/5G)
    const amf = Buffer.from([0x80, 0x00]);

    // Generate vectors
    const vectors = generateVectors(ki, op, sqnBuf, amf);

    // Increment SQN for next use (prevent replay attacks)
    // Only increment if we're using the subscriber's current SQN (not an override)
    if (sqnOverride === undefined) {
      await db
        .update(subscribers)
        .set({ sqn: sqnValue + 1, updatedAt: new Date() })
        .where(eq(subscribers.imsi, imsi));
    }

    // Zero sensitive buffers
    zeroBuffer(dek);
    zeroBuffer(ki);
    zeroBuffer(op);

    return {
      rand: vectors.rand.toString('base64'),
      autn: vectors.autn.toString('base64'),
      xres: vectors.xres.toString('base64'),
      ck: vectors.ck.toString('base64'),
      ik: vectors.ik.toString('base64'),
    };
  },
);

// POST /resync — resynchronize SQN after SYNC_FAILURE
app.post(
  '/resync',
  {
    schema: {
      body: ResyncRequest,
      response: {
        200: ResyncResponse,
        400: ErrorResponse,
        404: ErrorResponse,
      },
    },
  },
  async (request, reply) => {
    const { imsi, rand: randBase64, auts: autsBase64 } = request.body;

    // Look up subscriber
    const rows = await db
      .select()
      .from(subscribers)
      .where(eq(subscribers.imsi, imsi))
      .limit(1);

    const subscriber = rows[0];
    if (!subscriber) {
      return reply.status(404).send({ error: 'Subscriber not found' });
    }

    // Decode RAND and AUTS from base64
    const rand = Buffer.from(randBase64, 'base64');
    const auts = Buffer.from(autsBase64, 'base64');

    if (rand.length !== 16) {
      return reply.status(400).send({ error: 'RAND must be 16 bytes' });
    }
    if (auts.length !== 14) {
      return reply.status(400).send({ error: 'AUTS must be 14 bytes' });
    }

    // Unwrap DEK and decrypt Ki/OP
    const dek = await keyManager.unwrapDek(subscriber.kiDekWrapped);
    const ki = decryptWithDek(dek, subscriber.kiEncrypted);
    const op = decryptWithDek(dek, subscriber.opEncrypted);

    // Validate AUTS
    const result = validateAuts(ki, rand, auts, op);

    if (!result.valid || !result.sqnMs) {
      zeroBuffer(dek);
      zeroBuffer(ki);
      zeroBuffer(op);
      return reply.status(400).send({ error: 'AUTS validation failed' });
    }

    // Extract SQN_MS as a number (48-bit big-endian)
    const sqnMsValue = result.sqnMs.readUIntBE(0, 6);

    // Update subscriber's SQN to SQN_MS + delta (32 for margin)
    // This ensures the network's SQN is ahead of the device's
    const newSqn = sqnMsValue + 32;
    await db
      .update(subscribers)
      .set({ sqn: newSqn, updatedAt: new Date() })
      .where(eq(subscribers.imsi, imsi));

    // Build new SQN buffer (6 bytes, big-endian)
    const sqnBuf = Buffer.alloc(6);
    sqnBuf.writeUIntBE(newSqn, 0, 6);

    // AMF: 0x8000 (separation bit set for LTE/5G)
    const amf = Buffer.from([0x80, 0x00]);

    // Generate fresh vectors with updated SQN
    const vectors = generateVectors(ki, op, sqnBuf, amf);

    // Zero sensitive buffers
    zeroBuffer(dek);
    zeroBuffer(ki);
    zeroBuffer(op);

    return {
      rand: vectors.rand.toString('base64'),
      autn: vectors.autn.toString('base64'),
      xres: vectors.xres.toString('base64'),
      ck: vectors.ck.toString('base64'),
      ik: vectors.ik.toString('base64'),
    };
  },
);

// Verify Postgres connection on startup
app.addHook('onReady', async () => {
  const client = await pool.connect();
  try {
    await client.query('SELECT 1');
    app.log.info('Postgres connection verified');
  } finally {
    client.release();
  }
});

const start = async () => {
  try {
    await app.listen({ port: config.port, host: config.host });
  } catch (err) {
    app.log.error(err);
    process.exit(1);
  }
};

start();

export { app };
