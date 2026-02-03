import type { FastifyInstance } from 'fastify';
import { eq } from 'drizzle-orm';
import { EntitlementRequestBody, EntitlementRequestQuery } from '../../protocol/requestSchemas.js';
import { HTTP_STATUS } from '../../config/constants.js';
import { logger } from '../../config/logger.js';
import { db } from '../../db/index.js';
import { entitlements } from '../../db/schema.js';
import { handleInitialRequest, handleEapResponse } from '../../auth/eapAka.js';
import { HssSubscriberNotFoundError } from '../../auth/eapAkaVectors.js';
import { validateToken } from '../../auth/tokenService.js';
import { cacheResponse, getCachedResponse } from '../../auth/eapIdempotency.js';

export async function entitlementRoutes(app: FastifyInstance): Promise<void> {
  /**
   * POST /entitlement — three-path routing:
   *
   * 1. token present       → validate → 200 with entitlements
   * 2. eap_relay present   → EAP-AKA Round Trip 2 → 200 + token
   * 3. neither (initial)   → EAP-AKA Round Trip 1 → 401 + challenge
   */
  app.post(
    '/entitlement',
    {
      schema: {
        body: EntitlementRequestBody,
      },
    },
    async (request, reply) => {
      const body = request.body as {
        app: string;
        terminal_id: string;
        entitlement_version: string;
        imsi?: string;
        token?: string;
        eap_relay?: string;
      };

      const clientIp = request.ip;

      // --- Path 1: Fast Auth (token present) ---
      if (body.token) {
        const tokenInfo = await validateToken(body.token);
        if (!tokenInfo) {
          return reply.code(HTTP_STATUS.UNAUTHORIZED).send({
            error: 'Unauthorized',
            message: 'Invalid or expired token',
            statusCode: HTTP_STATUS.UNAUTHORIZED,
          });
        }

        const entitlementData = await fetchEntitlements(tokenInfo.subscriberId, body.app);
        return reply.code(HTTP_STATUS.OK).send({
          token: body.token,
          ...entitlementData,
        });
      }

      // --- Path 2: EAP-AKA Response (eap_relay present) ---
      if (body.eap_relay) {
        const sessionId = request.headers['x-eap-session-id'] as string | undefined;
        if (!sessionId) {
          return reply.code(HTTP_STATUS.BAD_REQUEST).send({
            error: 'Bad Request',
            message: 'Missing X-EAP-Session-Id header',
            statusCode: HTTP_STATUS.BAD_REQUEST,
          });
        }

        // Check idempotency cache
        const cached = await getCachedResponse(sessionId, body.eap_relay);
        if (cached) {
          logger.info({ sessionId }, 'Returning cached EAP response');
          return reply.code(HTTP_STATUS.OK).send(cached);
        }

        const result = await handleEapResponse(body.eap_relay, sessionId, clientIp);

        if (result.statusCode === 200 && result.token) {
          const entitlementData = await fetchEntitlements(result.subscriberId!, body.app);
          const response = {
            token: result.token,
            eap_relay: result.eapRelay,
            ...entitlementData,
          };

          // Cache success for idempotency
          await cacheResponse(sessionId, body.eap_relay, response);

          return reply.code(HTTP_STATUS.OK).send(response);
        }

        // Auth failed
        return reply.code(HTTP_STATUS.UNAUTHORIZED).send({
          error: 'Unauthorized',
          message: 'EAP-AKA authentication failed',
          eap_relay: result.eapRelay,
          statusCode: HTTP_STATUS.UNAUTHORIZED,
        });
      }

      // --- Path 3: Initial Request (no token, no eap_relay) ---
      if (!body.imsi) {
        return reply.code(HTTP_STATUS.BAD_REQUEST).send({
          error: 'Bad Request',
          message: 'IMSI is required for initial authentication',
          statusCode: HTTP_STATUS.BAD_REQUEST,
        });
      }

      try {
        const challenge = await handleInitialRequest(body.imsi, clientIp);

        return reply
          .code(HTTP_STATUS.UNAUTHORIZED)
          .header('X-EAP-Session-Id', challenge.sessionId)
          .send({
            eap_relay: challenge.eapRelay,
            statusCode: HTTP_STATUS.UNAUTHORIZED,
          });
      } catch (err) {
        if (err instanceof HssSubscriberNotFoundError) {
          return reply.code(HTTP_STATUS.FORBIDDEN).send({
            error: 'Forbidden',
            message: 'Unknown subscriber',
            statusCode: HTTP_STATUS.FORBIDDEN,
          });
        }
        throw err;
      }
    },
  );

  /**
   * GET /entitlement — requires a valid token.
   */
  app.get(
    '/entitlement',
    {
      schema: {
        querystring: EntitlementRequestQuery,
      },
    },
    async (request, reply) => {
      const query = request.query as {
        app: string;
        terminal_id: string;
        entitlement_version: string;
        token?: string;
      };

      if (!query.token) {
        return reply.code(HTTP_STATUS.UNAUTHORIZED).send({
          error: 'Unauthorized',
          message: 'Token is required',
          statusCode: HTTP_STATUS.UNAUTHORIZED,
        });
      }

      const tokenInfo = await validateToken(query.token);
      if (!tokenInfo) {
        return reply.code(HTTP_STATUS.UNAUTHORIZED).send({
          error: 'Unauthorized',
          message: 'Invalid or expired token',
          statusCode: HTTP_STATUS.UNAUTHORIZED,
        });
      }

      const entitlementData = await fetchEntitlements(tokenInfo.subscriberId, query.app);
      return reply.code(HTTP_STATUS.OK).send({
        token: query.token,
        ...entitlementData,
      });
    },
  );
}

/**
 * Fetch entitlement data for a subscriber and app.
 * Returns a default "enabled" response if no specific entitlement is configured.
 */
async function fetchEntitlements(
  subscriberId: string,
  appId: string,
): Promise<object> {
  const rows = await db
    .select()
    .from(entitlements)
    .where(eq(entitlements.subscriberId, subscriberId))
    .limit(10);

  const appEntitlement = rows.find((e) => e.appId === appId);

  if (appEntitlement) {
    return {
      entitlement: {
        appId: appEntitlement.appId,
        status: appEntitlement.status,
        provStatus: appEntitlement.provStatus,
        tcStatus: appEntitlement.tcStatus,
        configData: appEntitlement.configData,
      },
    };
  }

  // Default: service entitled but no specific config
  return {
    entitlement: {
      appId,
      status: 1, // ENABLED
      provStatus: 0, // NOT_NEEDED
      tcStatus: 0, // NOT_PROVIDED
    },
  };
}
