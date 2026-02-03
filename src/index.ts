// OpenTelemetry must be initialized before any other imports
import './config/tracing.js';

import 'dotenv/config';
import { buildApp } from './server/app.js';
import { config } from './config/index.js';
import { logger } from './config/logger.js';

async function main() {
  const app = await buildApp();

  try {
    await app.listen({ port: config.port, host: config.host });
    logger.info(
      { port: config.port, host: config.host },
      'Entitlement Configuration Server started',
    );
  } catch (err) {
    logger.fatal({ err }, 'Failed to start server');
    process.exit(1);
  }

  const shutdown = async (signal: string) => {
    logger.info({ signal }, 'Shutting down');
    await app.close();
    process.exit(0);
  };

  process.on('SIGTERM', () => shutdown('SIGTERM'));
  process.on('SIGINT', () => shutdown('SIGINT'));
}

main();
