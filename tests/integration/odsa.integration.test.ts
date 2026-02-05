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
 *
 * ODSA operations use temporary tokens (DB-backed) which bypass the
 * re-auth challenge flow. In production, a device first does a re-auth
 * exchange with AcquireTemporaryToken, then uses that temporary token
 * for subsequent ODSA operations.
 */

import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import crypto from 'node:crypto';
import { generateReauthId, storeReauthState } from '../../src/auth/reauthStore.js';
import { generateToken, findSubscriberByImsi } from '../../src/auth/tokenService.js';
import {
  decodeEapPacket,
  encodeEapPacket,
  EAP_CODE,
  EAP_TYPE_AKA,
  AKA_SUBTYPE,
  AT,
  type EapPacket,
} from '../../src/auth/eapCodec.js';
import { encryptAttributes, decryptAttributes } from '../../src/auth/eapEncryption.js';
import { computeMac } from '../../src/auth/keyDerivation.js';
import { TOKEN_TYPES } from '../../src/config/constants.js';

const TEST_IMSI_ALICE = '001010000000001';
const TEST_IMSI_BOB = '001010000000002';

let app: Awaited<ReturnType<typeof import('../../src/server/app.js').buildApp>>;

const BASE_BODY = {
  terminal_id: '12345678901234',
  entitlement_version: '2',
};

/**
 * Get a temporary token for a subscriber (simulates the result of
 * AcquireTemporaryToken). Uses generateToken directly since ODSA
 * operations work with DB-backed temporary tokens.
 */
async function getTokenForSubscriber(imsi: string): Promise<string> {
  const subscriberId = await findSubscriberByImsi(imsi);
  if (!subscriberId) throw new Error(`Subscriber not found: ${imsi}`);
  const token = await generateToken(subscriberId, TOKEN_TYPES.TEMPORARY, '127.0.0.1');
  return token.tokenValue;
}

/**
 * Helper to get a re-auth identity and keys, perform the re-auth exchange,
 * and return the new re-auth identity (for tests that need the full flow).
 */
async function performReauthExchange(
  imsi: string,
  appId: string,
  operation?: string,
): Promise<{ token: string; responseBody: Record<string, unknown> }> {
  const subscriberId = await findSubscriberByImsi(imsi);
  if (!subscriberId) throw new Error(`Subscriber not found: ${imsi}`);

  const mk = crypto.randomBytes(20);
  const kAut = crypto.randomBytes(16);
  const kEncr = crypto.randomBytes(16);
  const reauthId = generateReauthId();
  await storeReauthState({
    subscriberId,
    imsi,
    mk: mk.toString('base64'),
    kAut: kAut.toString('base64'),
    kEncr: kEncr.toString('base64'),
    counter: 1,
    identity: reauthId,
  });

  // RT1: POST with re-auth identity → 401 + challenge
  const rt1 = await app.inject({
    method: 'POST',
    url: '/entitlement',
    payload: {
      ...BASE_BODY,
      app: appId,
      token: reauthId,
      ...(operation ? { operation } : {}),
    },
  });

  if (rt1.statusCode !== 401) throw new Error(`Expected 401, got ${rt1.statusCode}`);
  const sessionId = rt1.headers['x-eap-session-id'] as string;

  // Build client response
  const eapResponse = buildReauthResponse(rt1.json().eap_relay, kAut, kEncr);

  // RT2: Send response → 200
  const rt2 = await app.inject({
    method: 'POST',
    url: '/entitlement',
    headers: { 'x-eap-session-id': sessionId },
    payload: {
      ...BASE_BODY,
      app: appId,
      eap_relay: eapResponse,
      ...(operation ? { operation } : {}),
    },
  });

  if (rt2.statusCode !== 200) throw new Error(`Expected 200, got ${rt2.statusCode}`);
  return { token: rt2.json().Token.token, responseBody: rt2.json() };
}

function buildReauthResponse(
  challengeBase64: string,
  kAut: Buffer,
  kEncr: Buffer,
): string {
  const challengeBytes = Buffer.from(challengeBase64, 'base64');
  const challenge = decodeEapPacket(challengeBytes);
  const atIv = challenge.attributes!.find((a) => a.type === AT.AT_IV)!;
  const atEncrData = challenge.attributes!.find((a) => a.type === AT.AT_ENCR_DATA)!;
  const innerAttrs = decryptAttributes(kEncr, atIv.value, atEncrData.value);
  const atCounter = innerAttrs.find((a) => a.type === AT.AT_COUNTER)!;

  const clientIv = crypto.randomBytes(16);
  const clientCiphertext = encryptAttributes(kEncr, clientIv, [
    { type: AT.AT_COUNTER, value: atCounter.value },
  ]);

  const responsePacket: EapPacket = {
    code: EAP_CODE.RESPONSE,
    identifier: challenge.identifier,
    type: EAP_TYPE_AKA,
    subtype: AKA_SUBTYPE.REAUTHENTICATION,
    attributes: [
      { type: AT.AT_IV, value: clientIv },
      { type: AT.AT_ENCR_DATA, value: clientCiphertext },
      { type: AT.AT_MAC, value: Buffer.alloc(16) },
    ],
  };

  const responseBytes = encodeEapPacket(responsePacket);
  let macOffset = 8;
  while (macOffset + 2 <= responseBytes.length) {
    if (responseBytes.readUInt8(macOffset) === AT.AT_MAC) break;
    macOffset += responseBytes.readUInt8(macOffset + 1) * 4;
  }
  const mac = computeMac(kAut, responseBytes);
  mac.copy(responseBytes, macOffset + 4);

  return responseBytes.toString('base64');
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

    it('AcquireTemporaryToken via re-auth → 200 with TemporaryToken in response', async () => {
      const { responseBody } = await performReauthExchange(
        TEST_IMSI_ALICE,
        'ap2006',
        'AcquireTemporaryToken',
      );

      expect(responseBody.ap2006).toBeDefined();
      const ap2006 = responseBody.ap2006 as Record<string, unknown>;
      expect(ap2006.TemporaryToken).toBeDefined();
      expect(ap2006.TemporaryTokenValidity).toBeDefined();
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
