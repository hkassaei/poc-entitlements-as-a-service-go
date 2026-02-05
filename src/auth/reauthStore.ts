/**
 * Re-auth State Store
 *
 * Long-lived Redis storage keyed by re-auth identity.
 * TTL = fastAuthTokenTtlSeconds (default 48h).
 *
 * Redis-only (no Postgres) — if lost, device falls back to full auth.
 */

import crypto from 'node:crypto';
import { redis } from '../db/redis.js';
import { config } from '../config/index.js';

export interface ReauthState {
  subscriberId: string;
  imsi: string;
  mk: string;      // base64
  kAut: string;     // base64
  kEncr: string;    // base64
  counter: number;
  identity: string; // the re-auth ID itself
}

function reauthKey(reauthId: string): string {
  return `reauth:${reauthId}`;
}

/**
 * Generate a cryptographically random re-auth identity.
 */
export function generateReauthId(): string {
  return crypto.randomBytes(24).toString('base64url');
}

/**
 * Store re-auth state in Redis with TTL.
 */
export async function storeReauthState(state: ReauthState): Promise<void> {
  const key = reauthKey(state.identity);
  const fields: Record<string, string> = {
    subscriberId: state.subscriberId,
    imsi: state.imsi,
    mk: state.mk,
    kAut: state.kAut,
    kEncr: state.kEncr,
    counter: String(state.counter),
    identity: state.identity,
  };
  await redis.hset(key, fields);
  await redis.expire(key, config.fastAuthTokenTtlSeconds);
}

/**
 * Retrieve re-auth state by re-auth identity.
 * Returns null if not found or expired.
 */
export async function getReauthState(reauthId: string): Promise<ReauthState | null> {
  const key = reauthKey(reauthId);
  const data = await redis.hgetall(key);

  if (!data || Object.keys(data).length === 0) {
    return null;
  }

  return {
    subscriberId: data.subscriberId!,
    imsi: data.imsi!,
    mk: data.mk!,
    kAut: data.kAut!,
    kEncr: data.kEncr!,
    counter: parseInt(data.counter!, 10),
    identity: data.identity!,
  };
}

/**
 * Delete re-auth state (after rotation or expiry).
 */
export async function deleteReauthState(reauthId: string): Promise<void> {
  await redis.del(reauthKey(reauthId));
}
