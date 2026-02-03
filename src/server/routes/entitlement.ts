import type { FastifyInstance } from 'fastify';
import { EntitlementRequestBody, EntitlementRequestQuery } from '../../protocol/requestSchemas.js';
import { HTTP_STATUS } from '../../config/constants.js';

export async function entitlementRoutes(app: FastifyInstance): Promise<void> {
  app.post(
    '/entitlement',
    {
      schema: {
        body: EntitlementRequestBody,
      },
    },
    async (_request, reply) => {
      // Placeholder: real logic comes in Phase 3+
      reply.code(HTTP_STATUS.NOT_IMPLEMENTED).send({
        error: 'Not Implemented',
        message: 'Entitlement service not yet implemented',
        statusCode: HTTP_STATUS.NOT_IMPLEMENTED,
      });
    },
  );

  app.get(
    '/entitlement',
    {
      schema: {
        querystring: EntitlementRequestQuery,
      },
    },
    async (_request, reply) => {
      // Placeholder: real logic comes in Phase 3+
      reply.code(HTTP_STATUS.NOT_IMPLEMENTED).send({
        error: 'Not Implemented',
        message: 'Entitlement service not yet implemented',
        statusCode: HTTP_STATUS.NOT_IMPLEMENTED,
      });
    },
  );
}
