import pg from 'pg';
import { drizzle } from 'drizzle-orm/node-postgres';
import {
  pgTable,
  uuid,
  varchar,
  bigint,
  timestamp,
  customType,
} from 'drizzle-orm/pg-core';
import { config } from './config.js';

// Minimal bytea custom type matching the main ECS schema
const bytea = customType<{ data: Buffer }>({
  dataType() {
    return 'bytea';
  },
});

// Minimal subscriber table definition — only fields needed by mock-hss
export const subscribers = pgTable('subscribers', {
  id: uuid('id').defaultRandom().primaryKey(),
  imsi: varchar('imsi', { length: 15 }).unique().notNull(),
  kiEncrypted: bytea('ki_encrypted').notNull(),
  opEncrypted: bytea('op_encrypted').notNull(),
  kiDekWrapped: bytea('ki_dek_wrapped').notNull(),
  sqn: bigint('sqn', { mode: 'number' }).default(0),
  createdAt: timestamp('created_at', { withTimezone: true }).defaultNow(),
  updatedAt: timestamp('updated_at', { withTimezone: true }).defaultNow(),
});

const pool = new pg.Pool({
  connectionString: config.databaseUrl,
  max: config.dbPoolSize,
});

export const db = drizzle(pool, { schema: { subscribers } });
export { pool };
