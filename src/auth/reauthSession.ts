/**
 * Re-auth Session Store
 *
 * Short-lived session for re-auth challenge/response correlation.
 * TTL = REAUTH_SESSION_TTL_SECONDS (90s).
 *
 * Redis key pattern: reauth_session:{sessionId}
 * Distinct from eap_session: used for full auth.
 */

import crypto from 'node:crypto';
import { redis } from '../db/redis.js';
import { EAP_AKA } from '../config/constants.js';

export interface ReauthSessionData {
  reauthId: string;
  nextReauthId: string;
  nonceS: string;      // base64
  counter: number;
  identifier: number;
  kAut: string;         // base64
  kEncr: string;        // base64
  mk: string;           // base64
  subscriberId: string;
  imsi: string;
}

function sessionKey(sessionId: string): string {
  return `reauth_session:${sessionId}`;
}

/**
 * Create a new re-auth session. Returns the generated session ID.
 */
export async function createReauthSession(data: ReauthSessionData): Promise<string> {
  const sessionId = crypto.randomUUID();
  const key = sessionKey(sessionId);

  const fields: Record<string, string> = {
    reauthId: data.reauthId,
    nextReauthId: data.nextReauthId,
    nonceS: data.nonceS,
    counter: String(data.counter),
    identifier: String(data.identifier),
    kAut: data.kAut,
    kEncr: data.kEncr,
    mk: data.mk,
    subscriberId: data.subscriberId,
    imsi: data.imsi,
  };

  await redis.hset(key, fields);
  await redis.expire(key, EAP_AKA.REAUTH_SESSION_TTL_SECONDS);

  return sessionId;
}

/**
 * Retrieve a re-auth session by session ID.
 * Returns null if not found or expired.
 */
export async function getReauthSession(sessionId: string): Promise<ReauthSessionData | null> {
  const key = sessionKey(sessionId);
  const data = await redis.hgetall(key);

  if (!data || Object.keys(data).length === 0) {
    return null;
  }

  return {
    reauthId: data.reauthId!,
    nextReauthId: data.nextReauthId!,
    nonceS: data.nonceS!,
    counter: parseInt(data.counter!, 10),
    identifier: parseInt(data.identifier!, 10),
    kAut: data.kAut!,
    kEncr: data.kEncr!,
    mk: data.mk!,
    subscriberId: data.subscriberId!,
    imsi: data.imsi!,
  };
}

/**
 * Delete a re-auth session.
 */
export async function deleteReauthSession(sessionId: string): Promise<void> {
  await redis.del(sessionKey(sessionId));
}
