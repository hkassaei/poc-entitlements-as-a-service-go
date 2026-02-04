/**
 * Token Service
 *
 * Generates and validates authentication tokens.
 * Tokens are stored in Postgres (source of truth) and cached in Redis.
 */

import crypto from 'node:crypto';
import { eq } from 'drizzle-orm';
import { db } from '../db/index.js';
import { redis } from '../db/redis.js';
import { subscribers, tokens } from '../db/schema.js';
import { config } from '../config/index.js';
import { TOKEN_TYPES } from '../config/constants.js';

export interface TokenInfo {
  tokenValue: string;
  subscriberId: string;
  tokenType: string;
  expiresAt: Date;
}

const TOKEN_CACHE_PREFIX = 'token:';

function tokenCacheKey(tokenValue: string): string {
  return `${TOKEN_CACHE_PREFIX}${tokenValue}`;
}

/**
 * Generate a new authentication token for a subscriber.
 *
 * - Inserts into Postgres tokens table
 * - Caches in Redis with appropriate TTL
 */
export async function generateToken(
  subscriberId: string,
  tokenType: string,
  clientIp: string,
): Promise<TokenInfo> {
  const tokenValue = crypto.randomBytes(32).toString('hex');

  let ttlSeconds: number;
  switch (tokenType) {
    case TOKEN_TYPES.FAST_AUTH:
      ttlSeconds = config.fastAuthTokenTtlSeconds;
      break;
    case TOKEN_TYPES.TEMPORARY:
      ttlSeconds = config.tempTokenTtlSeconds;
      break;
    default:
      ttlSeconds = config.authTokenTtlSeconds;
  }

  const expiresAt = new Date(Date.now() + ttlSeconds * 1000);

  await db.insert(tokens).values({
    subscriberId,
    tokenValue,
    tokenType,
    expiresAt,
    createdByIp: clientIp,
  });

  // Cache in Redis
  const cacheData = JSON.stringify({
    subscriberId,
    tokenType,
    expiresAt: expiresAt.toISOString(),
  });
  await redis.set(tokenCacheKey(tokenValue), cacheData, 'EX', ttlSeconds);

  return { tokenValue, subscriberId, tokenType, expiresAt };
}

/**
 * Validate a token. Checks Redis cache first, falls back to Postgres.
 *
 * Returns the subscriber ID if valid, null if invalid/expired.
 */
export async function validateToken(tokenValue: string): Promise<TokenInfo | null> {
  // Try Redis cache first
  const cached = await redis.get(tokenCacheKey(tokenValue));
  if (cached) {
    const data = JSON.parse(cached) as { subscriberId: string; tokenType: string; expiresAt: string };
    const expiresAt = new Date(data.expiresAt);
    if (expiresAt > new Date()) {
      return { tokenValue, subscriberId: data.subscriberId, tokenType: data.tokenType, expiresAt };
    }
    // Expired in cache — delete it
    await redis.del(tokenCacheKey(tokenValue));
  }

  // Fallback to Postgres
  const rows = await db
    .select()
    .from(tokens)
    .where(eq(tokens.tokenValue, tokenValue))
    .limit(1);

  const token = rows[0];
  if (!token) return null;

  if (token.expiresAt < new Date()) return null;
  if (token.consumed) return null;

  // Re-cache in Redis
  const remainingTtl = Math.floor((token.expiresAt.getTime() - Date.now()) / 1000);
  if (remainingTtl > 0) {
    const cacheData = JSON.stringify({
      subscriberId: token.subscriberId,
      tokenType: token.tokenType,
      expiresAt: token.expiresAt.toISOString(),
    });
    await redis.set(tokenCacheKey(tokenValue), cacheData, 'EX', remainingTtl);
  }

  return {
    tokenValue: token.tokenValue,
    subscriberId: token.subscriberId,
    tokenType: token.tokenType,
    expiresAt: token.expiresAt,
  };
}

/**
 * Revoke a token by deleting it from Redis and marking it consumed in Postgres.
 */
export async function revokeToken(tokenValue: string): Promise<void> {
  await redis.del(tokenCacheKey(tokenValue));
  await db
    .update(tokens)
    .set({ consumed: true })
    .where(eq(tokens.tokenValue, tokenValue));
}

/**
 * Rotate a token: revoke the old one and issue a new one.
 * Returns the new token info.
 */
export async function rotateToken(
  oldTokenValue: string,
  subscriberId: string,
  tokenType: string,
  clientIp: string,
): Promise<TokenInfo> {
  await revokeToken(oldTokenValue);
  return generateToken(subscriberId, tokenType, clientIp);
}

/**
 * Find a subscriber by IMSI. Returns the subscriber ID or null.
 */
export async function findSubscriberByImsi(imsi: string): Promise<string | null> {
  const rows = await db
    .select({ id: subscribers.id })
    .from(subscribers)
    .where(eq(subscribers.imsi, imsi))
    .limit(1);

  return rows[0]?.id ?? null;
}
