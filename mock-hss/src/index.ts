import Fastify from 'fastify';
import { Type } from '@sinclair/typebox';
import { TypeBoxTypeProvider } from '@fastify/type-provider-typebox';
import { eq } from 'drizzle-orm';
import { config } from './config.js';
import { db, pool, subscribers } from './db.js';
import { LocalKeyManager } from './kms.js';
import { decryptWithDek, zeroBuffer } from './kms.js';
import { generateVectors } from './milenage.js';

const keyManager = new LocalKeyManager(config.localKekHex);

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
    const dek = keyManager.unwrapDek(subscriber.kiDekWrapped);
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
