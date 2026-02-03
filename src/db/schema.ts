import {
  pgTable,
  uuid,
  varchar,
  bigint,
  boolean,
  inet,
  text,
  integer,
  jsonb,
  timestamp,
  bigserial,
  customType,
  unique,
} from 'drizzle-orm/pg-core';

const bytea = customType<{ data: Buffer }>({
  dataType() {
    return 'bytea';
  },
});

export const subscribers = pgTable('subscribers', {
  id: uuid('id').defaultRandom().primaryKey(),
  imsi: varchar('imsi', { length: 15 }).unique().notNull(),
  msisdn: varchar('msisdn', { length: 15 }),
  iccid: varchar('iccid', { length: 20 }),
  eid: varchar('eid', { length: 32 }),
  kiEncrypted: bytea('ki_encrypted').notNull(),
  opEncrypted: bytea('op_encrypted').notNull(),
  kiDekWrapped: bytea('ki_dek_wrapped').notNull(),
  sqn: bigint('sqn', { mode: 'number' }).default(0),
  createdAt: timestamp('created_at', { withTimezone: true }).defaultNow(),
  updatedAt: timestamp('updated_at', { withTimezone: true }).defaultNow(),
});

export const devices = pgTable('devices', {
  id: uuid('id').defaultRandom().primaryKey(),
  subscriberId: uuid('subscriber_id').references(() => subscribers.id),
  imei: varchar('imei', { length: 15 }).notNull(),
  vendor: varchar('vendor', { length: 128 }),
  model: varchar('model', { length: 128 }),
  swVersion: varchar('sw_version', { length: 128 }),
  deviceType: varchar('device_type', { length: 64 }),
  eid: varchar('eid', { length: 32 }),
  createdAt: timestamp('created_at', { withTimezone: true }).defaultNow(),
});

export const entitlements = pgTable(
  'entitlements',
  {
    id: uuid('id').defaultRandom().primaryKey(),
    subscriberId: uuid('subscriber_id')
      .notNull()
      .references(() => subscribers.id),
    appId: varchar('app_id', { length: 10 }).notNull(),
    status: integer('status').notNull().default(0),
    provStatus: integer('prov_status').default(0),
    tcStatus: integer('tc_status').default(0),
    configData: jsonb('config_data'),
    createdAt: timestamp('created_at', { withTimezone: true }).defaultNow(),
    updatedAt: timestamp('updated_at', { withTimezone: true }).defaultNow(),
  },
  (table) => [unique().on(table.subscriberId, table.appId)],
);

export const tokens = pgTable('tokens', {
  id: uuid('id').defaultRandom().primaryKey(),
  subscriberId: uuid('subscriber_id')
    .notNull()
    .references(() => subscribers.id),
  tokenValue: varchar('token_value', { length: 512 }).unique().notNull(),
  tokenType: varchar('token_type', { length: 32 }).notNull(),
  issuedAt: timestamp('issued_at', { withTimezone: true }).notNull().defaultNow(),
  expiresAt: timestamp('expires_at', { withTimezone: true }).notNull(),
  scope: varchar('scope', { length: 256 }),
  operationTargets: text('operation_targets').array(),
  consumed: boolean('consumed').default(false),
  createdByIp: inet('created_by_ip'),
});

export const auditLog = pgTable('audit_log', {
  id: bigserial('id', { mode: 'number' }).primaryKey(),
  timestamp: timestamp('timestamp', { withTimezone: true }).defaultNow(),
  subscriberId: uuid('subscriber_id'),
  appId: varchar('app_id', { length: 10 }),
  operation: varchar('operation', { length: 64 }),
  requestSummary: jsonb('request_summary'),
  responseCode: integer('response_code'),
  clientIp: inet('client_ip'),
  userAgent: text('user_agent'),
});
