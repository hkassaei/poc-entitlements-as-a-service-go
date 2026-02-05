import { describe, it, expect, beforeAll, afterAll, afterEach } from 'vitest';
import { eq } from 'drizzle-orm';
import { generateToken, validateToken, revokeToken } from '../../src/auth/tokenService.js';
import { db } from '../../src/db/index.js';
import { redis } from '../../src/db/redis.js';
import { tokens } from '../../src/db/schema.js';
import { TOKEN_TYPES } from '../../src/config/constants.js';

// Use the seeded test subscriber
let testSubscriberId: string;
const createdTokens: string[] = [];

beforeAll(async () => {
  process.env.DATABASE_URL = process.env.DATABASE_URL ?? 'postgresql://ecs:password@localhost:5432/entitlements';
  process.env.REDIS_URL = process.env.REDIS_URL ?? 'redis://localhost:6379';

  // Re-import to pick up env vars
  const { db: database } = await import('../../src/db/index.js');
  const { subscribers } = await import('../../src/db/schema.js');

  const rows = await database
    .select({ id: subscribers.id })
    .from(subscribers)
    .limit(1);

  testSubscriberId = rows[0]!.id;

  // Ensure Redis is connected
  const redisModule = await import('../../src/db/redis.js');
  if (redisModule.redis.status === 'wait') {
    await redisModule.redis.connect();
  }
});

afterEach(async () => {
  // Clean up tokens created during tests
  for (const tokenValue of createdTokens) {
    await redis.del(`token:${tokenValue}`);
    await db.delete(tokens).where(eq(tokens.tokenValue, tokenValue));
  }
  createdTokens.length = 0;
});

afterAll(async () => {
  await redis.quit();
});

function trackToken(tokenValue: string) {
  createdTokens.push(tokenValue);
}

describe('Token Service', () => {
  describe('revokeToken', () => {
    it('marks the token as consumed in Postgres', async () => {
      const token = await generateToken(testSubscriberId, TOKEN_TYPES.AUTH, '127.0.0.1');
      trackToken(token.tokenValue);

      await revokeToken(token.tokenValue);

      const rows = await db
        .select({ consumed: tokens.consumed })
        .from(tokens)
        .where(eq(tokens.tokenValue, token.tokenValue))
        .limit(1);

      expect(rows[0]!.consumed).toBe(true);
    });

    it('removes the token from Redis cache', async () => {
      const token = await generateToken(testSubscriberId, TOKEN_TYPES.AUTH, '127.0.0.1');
      trackToken(token.tokenValue);

      // Confirm it's in Redis
      const before = await redis.get(`token:${token.tokenValue}`);
      expect(before).not.toBeNull();

      await revokeToken(token.tokenValue);

      const after = await redis.get(`token:${token.tokenValue}`);
      expect(after).toBeNull();
    });

    it('makes the token fail validation', async () => {
      const token = await generateToken(testSubscriberId, TOKEN_TYPES.AUTH, '127.0.0.1');
      trackToken(token.tokenValue);

      // Valid before revocation
      const validBefore = await validateToken(token.tokenValue);
      expect(validBefore).not.toBeNull();

      await revokeToken(token.tokenValue);

      // Invalid after revocation
      const validAfter = await validateToken(token.tokenValue);
      expect(validAfter).toBeNull();
    });
  });

});
