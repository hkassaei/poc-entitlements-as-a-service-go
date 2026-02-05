import { describe, it, expect, beforeAll, afterAll, afterEach } from 'vitest';
import { eq } from 'drizzle-orm';
import { generateToken, validateToken } from '../../src/auth/tokenService.js';
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
  describe('generateToken', () => {
    it('creates a token in Postgres and Redis', async () => {
      const token = await generateToken(testSubscriberId, TOKEN_TYPES.AUTH, '127.0.0.1');
      trackToken(token.tokenValue);

      expect(token.tokenValue).toBeDefined();
      expect(token.subscriberId).toBe(testSubscriberId);
      expect(token.tokenType).toBe(TOKEN_TYPES.AUTH);

      // Check Redis
      const cached = await redis.get(`token:${token.tokenValue}`);
      expect(cached).not.toBeNull();

      // Check Postgres
      const rows = await db
        .select()
        .from(tokens)
        .where(eq(tokens.tokenValue, token.tokenValue))
        .limit(1);
      expect(rows.length).toBe(1);
    });
  });

  describe('validateToken', () => {
    it('returns token info for valid token', async () => {
      const token = await generateToken(testSubscriberId, TOKEN_TYPES.AUTH, '127.0.0.1');
      trackToken(token.tokenValue);

      const result = await validateToken(token.tokenValue);
      expect(result).not.toBeNull();
      expect(result!.subscriberId).toBe(testSubscriberId);
    });

    it('returns null for non-existent token', async () => {
      const result = await validateToken('nonexistent-token-value');
      expect(result).toBeNull();
    });
  });
});
