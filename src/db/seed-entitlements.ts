/**
 * Entitlement Seed Script
 *
 * Seeds entitlement records for test subscribers.
 * Run after the mock-hss seed has created the subscribers.
 *
 * Usage: npx tsx src/db/seed-entitlements.ts
 */

import pg from 'pg';

const databaseUrl = process.env.DATABASE_URL ?? 'postgresql://ecs:password@localhost:5432/entitlements';

interface EntitlementSeed {
  imsi: string;
  appId: string;
  status: number;
  provStatus: number;
  tcStatus: number;
  configData: object;
}

const seedData: EntitlementSeed[] = [
  // --- Subscriber 1: "Alice" (001010000000001) ---
  // VoWiFi: fully enabled and provisioned
  {
    imsi: '001010000000001',
    appId: 'ap2004',
    status: 1, // ENABLED
    provStatus: 3, // COMPLETE
    tcStatus: 1, // ACCEPTED
    configData: {
      addresses: [
        { addrType: '1', addr: 'epdg.operator.com' },
        { addrType: '1', addr: 'pcscf.operator.com' },
      ],
    },
  },
  // VoLTE: enabled with VoLTE + VoNR
  {
    imsi: '001010000000001',
    appId: 'ap2003',
    status: 1,
    provStatus: 3,
    tcStatus: 1,
    configData: {
      addresses: [
        { addrType: '1', addr: 'pcscf.operator.com' },
      ],
      volteEntitled: '1',
      vonrEntitled: '1',
    },
  },
  // SMSoIP: enabled
  {
    imsi: '001010000000001',
    appId: 'ap2005',
    status: 1,
    provStatus: 0, // NOT_NEEDED
    tcStatus: 0, // NOT_PROVIDED
    configData: {
      addresses: [
        { addrType: '1', addr: 'smsc.operator.com' },
      ],
    },
  },

  // ODSA Companion: active + smdpAddress → ManageSubscription → DOWNLOAD_PROFILE
  {
    imsi: '001010000000001',
    appId: 'ap2006',
    status: 1,
    provStatus: 3,
    tcStatus: 1,
    configData: {
      subscriptionState: 'active',
      smdpAddress: 'smdp.operator.com',
      profileType: 'companion',
    },
  },
  // ODSA Primary: eligible + serviceFlowUrl → CheckEligibility → eligible, AcquirePlan → CONTINUE_TO_WS
  {
    imsi: '001010000000001',
    appId: 'ap2009',
    status: 1,
    provStatus: 0,
    tcStatus: 0,
    configData: {
      subscriptionState: 'eligible',
      serviceFlowUrl: 'https://operator.com/plans/select',
      planId: 'PLAN-UNLIMITED-001',
      planName: 'Unlimited Plus',
    },
  },

  // --- Subscriber 2: "Bob" (001010000000002) ---
  // VoWiFi: disabled, needs T&C acceptance
  {
    imsi: '001010000000002',
    appId: 'ap2004',
    status: 0, // DISABLED
    provStatus: 1, // REQUIRED
    tcStatus: 2, // REQUIRES_ACCEPTANCE
    configData: {
      serviceFlowUrl: 'https://operator.com/terms/vowifi',
    },
  },
  // VoLTE: enabled
  {
    imsi: '001010000000002',
    appId: 'ap2003',
    status: 1,
    provStatus: 3,
    tcStatus: 1,
    configData: {
      addresses: [
        { addrType: '1', addr: 'pcscf.operator.com' },
      ],
      volteEntitled: '1',
      vonrEntitled: '0',
    },
  },
  // ODSA Companion: eligible + serviceFlowUrl → ManageSubscription → CONTINUE_TO_WS
  {
    imsi: '001010000000002',
    appId: 'ap2006',
    status: 1,
    provStatus: 0,
    tcStatus: 0,
    configData: {
      subscriptionState: 'eligible',
      serviceFlowUrl: 'https://operator.com/companion/setup',
    },
  },
  // ODSA Primary: active + smdpAddress → ManageSubscription → DOWNLOAD_PROFILE
  {
    imsi: '001010000000002',
    appId: 'ap2009',
    status: 1,
    provStatus: 3,
    tcStatus: 1,
    configData: {
      subscriptionState: 'active',
      smdpAddress: 'smdp.operator.com',
      profileType: 'default',
    },
  },
];

async function seed() {
  const client = new pg.Client({ connectionString: databaseUrl });

  try {
    await client.connect();

    for (const entry of seedData) {
      // Look up subscriber ID by IMSI
      const subResult = await client.query(
        'SELECT id FROM subscribers WHERE imsi = $1',
        [entry.imsi],
      );

      if (subResult.rows.length === 0) {
        console.warn(`Subscriber not found: ${entry.imsi} — skipping`);
        continue;
      }

      const subscriberId = subResult.rows[0].id;

      await client.query(
        `INSERT INTO entitlements (subscriber_id, app_id, status, prov_status, tc_status, config_data)
         VALUES ($1, $2, $3, $4, $5, $6)
         ON CONFLICT (subscriber_id, app_id) DO UPDATE SET
           status = EXCLUDED.status,
           prov_status = EXCLUDED.prov_status,
           tc_status = EXCLUDED.tc_status,
           config_data = EXCLUDED.config_data,
           updated_at = NOW()`,
        [subscriberId, entry.appId, entry.status, entry.provStatus, entry.tcStatus, JSON.stringify(entry.configData)],
      );

      console.log(`Seeded entitlement: ${entry.imsi} / ${entry.appId} (status=${entry.status})`);
    }

    console.log('Entitlement seed complete.');
  } finally {
    await client.end();
  }
}

seed().catch((err) => {
  console.error('Entitlement seed failed:', err);
  process.exit(1);
});
