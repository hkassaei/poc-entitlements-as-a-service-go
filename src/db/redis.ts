import Redis from 'ioredis';
import { config } from '../config/index.js';
import { logger } from '../config/logger.js';

export const redis = new Redis.default(config.redisUrl, {
  maxRetriesPerRequest: 3,
  lazyConnect: true,
});

redis.on('error', (err: Error) => {
  logger.error({ err }, 'Redis connection error');
});

redis.on('connect', () => {
  logger.info('Redis connected');
});
