/**
 * EAP Idempotency Cache
 *
 * Caches successful EAP-AKA responses in Redis so that replayed
 * Round Trip 2 requests return the same response without re-processing.
 *
 * Key: eap_idempotency:SHA-256(sessionId + eapRelay)
 * TTL: 90 seconds
 */

import crypto from 'node:crypto';
import { redis } from '../db/redis.js';
import { EAP_AKA } from '../config/constants.js';

const IDEMPOTENCY_PREFIX = 'eap_idempotency:';

function idempotencyKey(sessionId: string, eapRelay: string): string {
  const hash = crypto
    .createHash('sha256')
    .update(sessionId + eapRelay)
    .digest('hex');
  return `${IDEMPOTENCY_PREFIX}${hash}`;
}

/**
 * Cache a successful response for replay.
 */
export async function cacheResponse(
  sessionId: string,
  eapRelay: string,
  response: object,
): Promise<void> {
  const key = idempotencyKey(sessionId, eapRelay);
  await redis.set(key, JSON.stringify(response), 'EX', EAP_AKA.IDEMPOTENCY_TTL_SECONDS);
}

/**
 * Retrieve a cached response, or null if not found.
 */
export async function getCachedResponse(
  sessionId: string,
  eapRelay: string,
): Promise<object | null> {
  const key = idempotencyKey(sessionId, eapRelay);
  const cached = await redis.get(key);
  if (!cached) return null;
  return JSON.parse(cached) as object;
}
