import type { FastifyInstance } from 'fastify';
import { EntitlementRequestBody, EntitlementRequestQuery } from '../../protocol/requestSchemas.js';
import { HTTP_STATUS } from '../../config/constants.js';
import { logger } from '../../config/logger.js';
import { handleInitialRequest, handleEapResponse } from '../../auth/eapAka.js';
import { HssSubscriberNotFoundError } from '../../auth/eapAkaVectors.js';
import { validateToken, rotateToken } from '../../auth/tokenService.js';
import { cacheResponse, getCachedResponse } from '../../auth/eapIdempotency.js';
import { buildEntitlementResponse } from '../../protocol/responseBuilder.js';

export async function entitlementRoutes(app: FastifyInstance): Promise<void> {
  /**
   * POST /entitlement — three-path routing:
   *
   * 1. token present       → validate → rotate → 200 with TS.43 response
   * 2. eap_relay present   → EAP-AKA Round Trip 2 → 200 + token + TS.43 response
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
        accept_content_type?: string;
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

        // Rotate: revoke old token, issue new one (rolling expiry)
        const newToken = await rotateToken(
          body.token,
          tokenInfo.subscriberId,
          tokenInfo.tokenType,
          clientIp,
        );

        const formatted = await buildEntitlementResponse(
          newToken.tokenValue,
          tokenInfo.subscriberId,
          body.app,
          body.accept_content_type,
        );

        return reply
          .code(HTTP_STATUS.OK)
          .type(formatted.contentType)
          .send(formatted.body);
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
          const formatted = await buildEntitlementResponse(
            result.token,
            result.subscriberId!,
            body.app,
            body.accept_content_type,
          );

          // For idempotency cache, always store the JSON representation
          const cacheData = typeof formatted.body === 'string'
            ? { _xml: formatted.body, eap_relay: result.eapRelay }
            : { ...(formatted.body as object), eap_relay: result.eapRelay };

          await cacheResponse(sessionId, body.eap_relay, cacheData);

          if (typeof formatted.body === 'string') {
            // XML response — prepend eap_relay info is not applicable in XML
            // Return the XML body directly
            return reply
              .code(HTTP_STATUS.OK)
              .type(formatted.contentType)
              .send(formatted.body);
          }

          return reply.code(HTTP_STATUS.OK).send({
            ...(formatted.body as object),
            eap_relay: result.eapRelay,
          });
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
   * GET /entitlement — requires a valid token. Read-only, no rotation.
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
        accept_content_type?: string;
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

      const formatted = await buildEntitlementResponse(
        query.token,
        tokenInfo.subscriberId,
        query.app,
        query.accept_content_type,
      );

      return reply
        .code(HTTP_STATUS.OK)
        .type(formatted.contentType)
        .send(formatted.body);
    },
  );
}
