/**
 * EAP Session Manager
 *
 * Manages EAP-AKA session state in Redis hashes.
 * Key pattern: eap_session:{sessionId} with 90s TTL.
 *
 * Session fields are stored as base64 strings for binary data.
 */

import crypto from 'node:crypto';
import { redis } from '../db/redis.js';
import { EAP_AKA, EAP_STATE } from '../config/constants.js';

export interface EapSessionData {
  imsi: string;
  state: string;
  rand: string;   // base64
  xres: string;   // base64
  ck: string;      // base64
  ik: string;      // base64
  identifier: string; // numeric string
  kAut: string;    // base64
  kEncr: string;   // base64
  mk: string;      // base64
}

function sessionKey(sessionId: string): string {
  return `eap_session:${sessionId}`;
}

/**
 * Create a new EAP session with a random session ID.
 * Returns the generated session ID.
 */
export async function createSession(data: {
  imsi: string;
  rand: Buffer;
  xres: Buffer;
  ck: Buffer;
  ik: Buffer;
  identifier: number;
  kAut: Buffer;
  kEncr: Buffer;
  mk: Buffer;
}): Promise<string> {
  const sessionId = crypto.randomUUID();
  const key = sessionKey(sessionId);

  const fields: EapSessionData = {
    imsi: data.imsi,
    state: EAP_STATE.CHALLENGE_SENT,
    rand: data.rand.toString('base64'),
    xres: data.xres.toString('base64'),
    ck: data.ck.toString('base64'),
    ik: data.ik.toString('base64'),
    identifier: String(data.identifier),
    kAut: data.kAut.toString('base64'),
    kEncr: data.kEncr.toString('base64'),
    mk: data.mk.toString('base64'),
  };

  await redis.hset(key, fields as unknown as Record<string, string>);
  await redis.expire(key, EAP_AKA.SESSION_TTL_SECONDS);

  return sessionId;
}

/**
 * Retrieve an EAP session by session ID.
 * Returns null if the session does not exist or has expired.
 */
export async function getSession(sessionId: string): Promise<EapSessionData | null> {
  const key = sessionKey(sessionId);
  const data = await redis.hgetall(key);

  if (!data || Object.keys(data).length === 0) {
    return null;
  }

  return data as unknown as EapSessionData;
}

/**
 * Update the state of an existing session.
 */
export async function updateSessionState(
  sessionId: string,
  state: string,
): Promise<void> {
  const key = sessionKey(sessionId);
  await redis.hset(key, 'state', state);
}

/**
 * Delete an EAP session (after successful auth or failure).
 */
export async function deleteSession(sessionId: string): Promise<void> {
  await redis.del(sessionKey(sessionId));
}

/**
 * Update session fields (used after SQN resync to store new vectors/keys).
 * Resets the TTL to the full session timeout.
 */
export async function updateSession(
  sessionId: string,
  updates: {
    rand?: Buffer;
    xres?: Buffer;
    ck?: Buffer;
    ik?: Buffer;
    identifier?: number;
    kAut?: Buffer;
    kEncr?: Buffer;
    mk?: Buffer;
  },
): Promise<void> {
  const key = sessionKey(sessionId);

  const fields: Record<string, string> = {};

  if (updates.rand) fields.rand = updates.rand.toString('base64');
  if (updates.xres) fields.xres = updates.xres.toString('base64');
  if (updates.ck) fields.ck = updates.ck.toString('base64');
  if (updates.ik) fields.ik = updates.ik.toString('base64');
  if (updates.identifier !== undefined) fields.identifier = String(updates.identifier);
  if (updates.kAut) fields.kAut = updates.kAut.toString('base64');
  if (updates.kEncr) fields.kEncr = updates.kEncr.toString('base64');
  if (updates.mk) fields.mk = updates.mk.toString('base64');

  if (Object.keys(fields).length > 0) {
    await redis.hset(key, fields);
    await redis.expire(key, EAP_AKA.SESSION_TTL_SECONDS);
  }
}
