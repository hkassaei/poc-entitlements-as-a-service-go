import crypto from 'node:crypto';
import pg from 'pg';
import { config } from './config.js';
import { encryptWithDek, wrapDek } from './kms.js';

// 3GPP TS 35.207 test vectors
const testSubscribers = [
  {
    imsi: '001010000000001',
    ki: '465b5ce8b199b49faa5f0a2ee238a6bc',
    op: 'cdc202d5123e20f62b6d676ac72cb318',
  },
  {
    imsi: '001010000000002',
    ki: '0396eb317b6d1c36f19c1c84cd6ffd16',
    op: 'ff53bade17df5d4e793073ce9d7579fa',
  },
];

async function seed() {
  const kek = Buffer.from(config.localKekHex, 'hex');
  const client = new pg.Client({ connectionString: config.databaseUrl });

  try {
    await client.connect();

    // Create table if not exists (raw DDL for bootstrapping)
    await client.query(`
      CREATE TABLE IF NOT EXISTS subscribers (
        id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
        imsi VARCHAR(15) UNIQUE NOT NULL,
        msisdn VARCHAR(15),
        iccid VARCHAR(20),
        eid VARCHAR(32),
        ki_encrypted BYTEA NOT NULL,
        op_encrypted BYTEA NOT NULL,
        ki_dek_wrapped BYTEA NOT NULL,
        sqn BIGINT DEFAULT 0,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);

    for (const sub of testSubscribers) {
      // Generate a random DEK for each subscriber
      const dek = crypto.randomBytes(32);

      // Encrypt Ki and OP with the DEK
      const kiPlain = Buffer.from(sub.ki, 'hex');
      const opPlain = Buffer.from(sub.op, 'hex');
      const kiEncrypted = encryptWithDek(dek, kiPlain);
      const opEncrypted = encryptWithDek(dek, opPlain);

      // Wrap the DEK with the KEK
      const dekWrapped = wrapDek(kek, dek);

      // Zero plaintext key material
      dek.fill(0);
      kiPlain.fill(0);
      opPlain.fill(0);

      // Upsert subscriber
      await client.query(
        `INSERT INTO subscribers (imsi, ki_encrypted, op_encrypted, ki_dek_wrapped, sqn)
         VALUES ($1, $2, $3, $4, 0)
         ON CONFLICT (imsi) DO UPDATE SET
           ki_encrypted = EXCLUDED.ki_encrypted,
           op_encrypted = EXCLUDED.op_encrypted,
           ki_dek_wrapped = EXCLUDED.ki_dek_wrapped,
           updated_at = NOW()`,
        [sub.imsi, kiEncrypted, opEncrypted, dekWrapped],
      );

      console.log(`Seeded subscriber: ${sub.imsi}`);
    }

    console.log('Seed complete.');
  } finally {
    await client.end();
  }
}

seed().catch((err) => {
  console.error('Seed failed:', err);
  process.exit(1);
});
