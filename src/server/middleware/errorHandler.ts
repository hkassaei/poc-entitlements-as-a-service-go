import type { FastifyError, FastifyReply, FastifyRequest } from 'fastify';
import { logger } from '../../config/logger.js';
import { HTTP_STATUS } from '../../config/constants.js';

export function errorHandler(
  error: FastifyError,
  request: FastifyRequest,
  reply: FastifyReply,
): void {
  const statusCode = error.statusCode ?? HTTP_STATUS.INTERNAL_SERVER_ERROR;

  if (statusCode >= 500) {
    logger.error(
      { err: error, method: request.method, url: request.url },
      'Internal server error',
    );
  } else {
    logger.warn(
      { err: error, method: request.method, url: request.url },
      'Client error',
    );
  }

  reply.code(statusCode).send({
    error: error.name ?? 'Error',
    message: error.message,
    statusCode,
    ...(error.validation ? { validation: error.validation } : {}),
  });
}
