import Fastify from 'fastify';
import { TypeBoxTypeProvider } from '@fastify/type-provider-typebox';
import { logger } from '../config/logger.js';
import { requestParser } from './middleware/requestParser.js';
import { userAgentParser } from './middleware/userAgent.js';
import { versionCheck } from './middleware/versionCheck.js';
import { errorHandler } from './middleware/errorHandler.js';
import { auditLogger } from './middleware/auditLogger.js';
import { healthRoutes } from './routes/health.js';
import { entitlementRoutes } from './routes/entitlement.js';
import { pool } from '../db/index.js';
import { redis } from '../db/redis.js';

export async function buildApp() {
  const app = Fastify({
    logger: false, // We use our own pino instance
  }).withTypeProvider<TypeBoxTypeProvider>();

  // Register hooks in order
  app.addHook('onRequest', requestParser);
  app.addHook('onRequest', userAgentParser);
  app.addHook('onRequest', versionCheck);

  // Audit logging (fires after response is sent)
  app.addHook('onResponse', auditLogger);

  // Centralized error handler
  app.setErrorHandler(errorHandler);

  // Register routes
  await app.register(healthRoutes);
  await app.register(entitlementRoutes);

  // Warm connections on ready
  app.addHook('onReady', async () => {
    try {
      const client = await pool.connect();
      await client.query('SELECT 1');
      client.release();
      logger.info('PostgreSQL connection verified');
    } catch (err) {
      logger.error({ err }, 'PostgreSQL connection failed on startup');
      throw err;
    }

    try {
      await redis.connect();
      await redis.ping();
      logger.info('Redis connection verified');
    } catch (err) {
      logger.error({ err }, 'Redis connection failed on startup');
      throw err;
    }
  });

  return app;
}
