/**
 * Integration Test Setup
 *
 * Ensures the database is seeded with entitlement test data before
 * integration tests run. This removes the dependency on manually
 * running `npm run db:seed-entitlements` before testing.
 */

import { seedEntitlements } from '../../src/db/seed-entitlements.js';

const databaseUrl = process.env.DATABASE_URL ?? 'postgresql://ecs:password@localhost:5432/entitlements';

export async function setup() {
  await seedEntitlements(databaseUrl);
}
