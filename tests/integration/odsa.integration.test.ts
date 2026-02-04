/**
 * ODSA Integration Tests
 *
 * These tests require Docker services to be running:
 *   docker compose up -d postgres redis mock-hss
 *
 * Entitlement seed data is applied automatically via vitest globalSetup.
 *
 * They exercise the full ODSA (On-Device Service Activation) flows
 * for companion (ap2006) and primary (ap2009) devices.
 */

import { describe, it, expect, beforeAll, afterAll } from 'vitest';

const TEST_IMSI_ALICE = '001010000000001';
const TEST_IMSI_BOB = '001010000000002';

let app: Awaited<ReturnType<typeof import('../../src/server/app.js').buildApp>>;

const BASE_BODY = {
  terminal_id: '12345678901234',
  entitlement_version: '2',
};

async function getTokenForSubscriber(imsi: string): Promise<string> {
  const { generateToken, findSubscriberByImsi } = await import('../../src/auth/tokenService.js');
  const subscriberId = await findSubscriberByImsi(imsi);
  if (!subscriberId) throw new Error(`Subscriber not found: ${imsi}`);
  const token = await generateToken(subscriberId, 'auth', '127.0.0.1');
  return token.tokenValue;
}

beforeAll(async () => {
  process.env.DATABASE_URL = process.env.DATABASE_URL ?? 'postgresql://ecs:password@localhost:5432/entitlements';
  process.env.REDIS_URL = process.env.REDIS_URL ?? 'redis://localhost:6379';
  process.env.HSS_URL = process.env.HSS_URL ?? 'http://localhost:3001';

  const { buildApp } = await import('../../src/server/app.js');
  app = await buildApp();
  await app.ready();
});

afterAll(async () => {
  await app.close();
});

describe('ODSA Integration', () => {
  describe('Companion Device (ap2006)', () => {
    it('CheckEligibility → 200 with SubscriptionResult', async () => {
      const token = await getTokenForSubscriber(TEST_IMSI_ALICE);

      const res = await app.inject({
        method: 'POST',
        url: '/entitlement',
        payload: {
          ...BASE_BODY,
          app: 'ap2006',
          token,
          operation: 'CheckEligibility',
        },
      });

      expect(res.statusCode).toBe(200);
      const body = res.json();
      expect(body.ap2006).toBeDefined();
      expect(body.ap2006.SubscriptionResult).toBeDefined();
    });

    it('ManageSubscription for active subscriber → DOWNLOAD_PROFILE with SM-DP+ data', async () => {
      const token = await getTokenForSubscriber(TEST_IMSI_ALICE);

      const res = await app.inject({
        method: 'POST',
        url: '/entitlement',
        payload: {
          ...BASE_BODY,
          app: 'ap2006',
          token,
          operation: 'ManageSubscription',
        },
      });

      expect(res.statusCode).toBe(200);
      const body = res.json();
      expect(body.ap2006.SubscriptionResult).toBe('2'); // DOWNLOAD_PROFILE
      expect(body.ap2006['SMDP+Address']).toBeDefined();
      expect(body.ap2006['SMDP+ActivationCode']).toBeDefined();
      expect(body.ap2006.ProfileICCID).toBeDefined();
    });

    it('ManageSubscription for eligible subscriber → CONTINUE_TO_WS with ServiceFlow_URL', async () => {
      const token = await getTokenForSubscriber(TEST_IMSI_BOB);

      const res = await app.inject({
        method: 'POST',
        url: '/entitlement',
        payload: {
          ...BASE_BODY,
          app: 'ap2006',
          token,
          operation: 'ManageSubscription',
        },
      });

      expect(res.statusCode).toBe(200);
      const body = res.json();
      expect(body.ap2006.SubscriptionResult).toBe('1'); // CONTINUE_TO_WS
      expect(body.ap2006.ServiceFlow_URL).toBeDefined();
    });

    it('AcquireTemporaryToken → 200 with TemporaryToken in response', async () => {
      const token = await getTokenForSubscriber(TEST_IMSI_ALICE);

      const res = await app.inject({
        method: 'POST',
        url: '/entitlement',
        payload: {
          ...BASE_BODY,
          app: 'ap2006',
          token,
          operation: 'AcquireTemporaryToken',
        },
      });

      expect(res.statusCode).toBe(200);
      const body = res.json();
      expect(body.ap2006).toBeDefined();
      expect(body.ap2006.TemporaryToken).toBeDefined();
      expect(body.ap2006.TemporaryTokenValidity).toBeDefined();
    });
  });

  describe('Primary Device (ap2009)', () => {
    it('AcquirePlan → CONTINUE_TO_WS with plan info', async () => {
      const token = await getTokenForSubscriber(TEST_IMSI_ALICE);

      const res = await app.inject({
        method: 'POST',
        url: '/entitlement',
        payload: {
          ...BASE_BODY,
          app: 'ap2009',
          token,
          operation: 'AcquirePlan',
        },
      });

      expect(res.statusCode).toBe(200);
      const body = res.json();
      expect(body.ap2009.SubscriptionResult).toBe('1'); // CONTINUE_TO_WS
      expect(body.ap2009.ServiceFlow_URL).toBeDefined();
      expect(body.ap2009.PlanId).toBeDefined();
      expect(body.ap2009.PlanName).toBeDefined();
    });

    it('ManageSubscription for active subscriber → DOWNLOAD_PROFILE', async () => {
      const token = await getTokenForSubscriber(TEST_IMSI_BOB);

      const res = await app.inject({
        method: 'POST',
        url: '/entitlement',
        payload: {
          ...BASE_BODY,
          app: 'ap2009',
          token,
          operation: 'ManageSubscription',
        },
      });

      expect(res.statusCode).toBe(200);
      const body = res.json();
      expect(body.ap2009.SubscriptionResult).toBe('2'); // DOWNLOAD_PROFILE
      expect(body.ap2009['SMDP+Address']).toBeDefined();
    });
  });

  describe('Content Type', () => {
    it('XML response for ODSA includes SubscriptionResult in WAP-Provisioning format', async () => {
      const token = await getTokenForSubscriber(TEST_IMSI_ALICE);

      const res = await app.inject({
        method: 'POST',
        url: '/entitlement',
        payload: {
          ...BASE_BODY,
          app: 'ap2006',
          token,
          operation: 'CheckEligibility',
          accept_content_type: 'xml',
        },
      });

      expect(res.statusCode).toBe(200);
      expect(res.headers['content-type']).toContain('application/xml');
      const xml = res.body;
      expect(xml).toContain('name="AppID" value="ap2006"');
      expect(xml).toContain('name="SubscriptionResult"');
    });
  });

  describe('Error Cases', () => {
    it('unknown operation → basic status response', async () => {
      const token = await getTokenForSubscriber(TEST_IMSI_ALICE);

      // Send request without operation — should get basic status
      const res = await app.inject({
        method: 'POST',
        url: '/entitlement',
        payload: {
          ...BASE_BODY,
          app: 'ap2006',
          token,
        },
      });

      expect(res.statusCode).toBe(200);
      const body = res.json();
      expect(body.ap2006).toBeDefined();
      expect(body.ap2006.EntitlementStatus).toBeDefined();
      // No SubscriptionResult when no operation is specified
      expect(body.ap2006.SubscriptionResult).toBeUndefined();
    });

    it('no entitlement for subscriber+app → uses defaults', async () => {
      const token = await getTokenForSubscriber(TEST_IMSI_BOB);

      // Bob has no ap2005 (SMSoIP) entitlement — tests the default fallback
      const res = await app.inject({
        method: 'POST',
        url: '/entitlement',
        payload: {
          ...BASE_BODY,
          app: 'ap2005',
          token,
        },
      });

      expect(res.statusCode).toBe(200);
      const body = res.json();
      expect(body.ap2005).toBeDefined();
      expect(body.ap2005.EntitlementStatus).toBe('1'); // default ENABLED
    });
  });
});
