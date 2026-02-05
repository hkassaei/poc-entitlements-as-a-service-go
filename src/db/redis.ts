import Redis from 'ioredis';
import { config } from '../config/index.js';
import { logger } from '../config/logger.js';

export const redis = new Redis.default(config.redisUrl, {
  maxRetriesPerRequest: 3,
  lazyConnect: true,
  connectTimeout: 10000,
  commandTimeout: 5000,
  retryStrategy(times: number) {
    if (times > 10) {
      logger.error('Redis retry limit reached, stopping reconnection');
      return null; // stop retrying
    }
    // Exponential backoff: 50ms, 100ms, 200ms, ... capped at 3s
    return Math.min(times * 50, 3000);
  },
  reconnectOnError(err: Error) {
    // Reconnect on connection reset (common after VPC network blips)
    return err.message.includes('ECONNRESET');
  },
});

redis.on('error', (err: Error) => {
  logger.error({ err }, 'Redis connection error');
});

redis.on('connect', () => {
  logger.info('Redis connected');
});
