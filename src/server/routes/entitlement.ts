import type { FastifyInstance } from 'fastify';
import { EntitlementRequestBody, EntitlementRequestQuery } from '../../protocol/requestSchemas.js';
import { HTTP_STATUS } from '../../config/constants.js';
import { logger } from '../../config/logger.js';
import { handleInitialRequest, handleEapResponse } from '../../auth/eapAka.js';
import { handleReauthRequest, handleReauthResponse } from '../../auth/eapReauth.js';
import { getReauthState } from '../../auth/reauthStore.js';
import { getReauthSession } from '../../auth/reauthSession.js';
import { HssSubscriberNotFoundError } from '../../auth/eapAkaVectors.js';
import { validateToken, generateTemporaryToken } from '../../auth/tokenService.js';
import { cacheResponse, getCachedResponse } from '../../auth/eapIdempotency.js';
import { buildEntitlementResponse } from '../../protocol/responseBuilder.js';

export async function entitlementRoutes(app: FastifyInstance): Promise<void> {
  /**
   * POST /entitlement — four-path routing:
   *
   * 1. eap_relay present   → re-auth RT2 (if reauth session) or full auth RT2
   * 2. token present        → re-auth RT1 (if reauth state) or ODSA temporary token flow
   * 3. neither (initial)    → EAP-AKA Round Trip 1 → 401 + challenge
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
        operation?: string;
        operation_type?: number;
      };

      const clientIp = request.ip;

      // --- Path 1: EAP Response (eap_relay present) ---
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

        // Path 1a: Try re-auth session first
        const reauthSession = await getReauthSession(sessionId);
        if (reauthSession) {
          const reauthResult = await handleReauthResponse(body.eap_relay, sessionId, clientIp);

          if (reauthResult.statusCode === 200 && reauthResult.reauthId) {
            const odsaCtx = body.operation
              ? { operation: body.operation, operationType: body.operation_type }
              : undefined;

            const formatted = await buildEntitlementResponse(
              reauthResult.reauthId,
              reauthResult.subscriberId!,
              body.app,
              body.accept_content_type,
              odsaCtx,
            );

            // Handle AcquireTemporaryToken side effect
            if (
              body.operation === 'AcquireTemporaryToken' &&
              (body.app === 'ap2006' || body.app === 'ap2009')
            ) {
              const tempToken = await generateTemporaryToken(
                reauthResult.subscriberId!,
                clientIp,
                body.app,
                [body.operation],
              );
              if (typeof formatted.body === 'object' && formatted.body !== null) {
                const appBlock = (formatted.body as Record<string, unknown>)[body.app] as
                  | Record<string, unknown>
                  | undefined;
                if (appBlock) {
                  appBlock.TemporaryToken = tempToken.tokenValue;
                  appBlock.TemporaryTokenValidity = String(
                    Math.floor((tempToken.expiresAt.getTime() - Date.now()) / 1000),
                  );
                }
              }
            }

            // Cache the response
            const cacheData = typeof formatted.body === 'string'
              ? { _xml: formatted.body, eap_relay: reauthResult.eapRelay }
              : { ...(formatted.body as object), eap_relay: reauthResult.eapRelay };
            await cacheResponse(sessionId, body.eap_relay, cacheData);

            if (typeof formatted.body === 'string') {
              return reply
                .code(HTTP_STATUS.OK)
                .type(formatted.contentType)
                .send(formatted.body);
            }

            return reply.code(HTTP_STATUS.OK).send({
              ...(formatted.body as object),
              eap_relay: reauthResult.eapRelay,
            });
          }

          // Counter-too-small fallback: re-auth returned 401 + new sessionId for full auth
          if (reauthResult.statusCode === 401 && reauthResult.sessionId) {
            return reply
              .code(HTTP_STATUS.UNAUTHORIZED)
              .header('X-EAP-Session-Id', reauthResult.sessionId)
              .send({
                eap_relay: reauthResult.eapRelay,
                statusCode: HTTP_STATUS.UNAUTHORIZED,
              });
          }

          // Re-auth failed
          return reply.code(HTTP_STATUS.UNAUTHORIZED).send({
            error: 'Unauthorized',
            message: 'Re-authentication failed',
            eap_relay: reauthResult.eapRelay,
            statusCode: HTTP_STATUS.UNAUTHORIZED,
          });
        }

        // Path 1b: Full auth RT2 (existing flow)
        const result = await handleEapResponse(body.eap_relay, sessionId, clientIp);

        if (result.statusCode === 200 && result.token) {
          const odsaCtx = body.operation
            ? { operation: body.operation, operationType: body.operation_type }
            : undefined;

          const formatted = await buildEntitlementResponse(
            result.token,
            result.subscriberId!,
            body.app,
            body.accept_content_type,
            odsaCtx,
          );

          // For idempotency cache, always store the JSON representation
          const cacheData = typeof formatted.body === 'string'
            ? { _xml: formatted.body, eap_relay: result.eapRelay }
            : { ...(formatted.body as object), eap_relay: result.eapRelay };

          await cacheResponse(sessionId, body.eap_relay, cacheData);

          if (typeof formatted.body === 'string') {
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

        // SQN resync: new challenge issued (same session ID)
        if (result.statusCode === 401 && result.sessionId) {
          return reply
            .code(HTTP_STATUS.UNAUTHORIZED)
            .header('X-EAP-Session-Id', result.sessionId)
            .send({
              eap_relay: result.eapRelay,
              statusCode: HTTP_STATUS.UNAUTHORIZED,
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

      // --- Path 2: Token present (re-auth RT1 or ODSA temporary token) ---
      if (body.token) {
        // Path 2a: Try re-auth state first (re-auth identity → initiate re-auth challenge)
        const reauthState = await getReauthState(body.token);
        if (reauthState) {
          const reauthChallenge = await handleReauthRequest(body.token, clientIp);
          if (reauthChallenge) {
            return reply
              .code(HTTP_STATUS.UNAUTHORIZED)
              .header('X-EAP-Session-Id', reauthChallenge.sessionId)
              .send({
                eap_relay: reauthChallenge.eapRelay,
                statusCode: HTTP_STATUS.UNAUTHORIZED,
              });
          }
          // Re-auth state found but counter exhausted — fall through to check ODSA token
        }

        // Path 2b: ODSA temporary token (existing flow)
        const tokenInfo = await validateToken(body.token);
        if (!tokenInfo) {
          return reply.code(HTTP_STATUS.UNAUTHORIZED).send({
            error: 'Unauthorized',
            message: 'Invalid or expired token',
            statusCode: HTTP_STATUS.UNAUTHORIZED,
          });
        }

        const odsaContext = body.operation
          ? { operation: body.operation, operationType: body.operation_type }
          : undefined;

        const formatted = await buildEntitlementResponse(
          body.token,
          tokenInfo.subscriberId,
          body.app,
          body.accept_content_type,
          odsaContext,
        );

        // Handle AcquireTemporaryToken side effect
        if (
          body.operation === 'AcquireTemporaryToken' &&
          (body.app === 'ap2006' || body.app === 'ap2009')
        ) {
          const tempToken = await generateTemporaryToken(
            tokenInfo.subscriberId,
            clientIp,
            body.app,
            [body.operation],
          );
          if (typeof formatted.body === 'object' && formatted.body !== null) {
            const appBlock = (formatted.body as Record<string, unknown>)[body.app] as
              | Record<string, unknown>
              | undefined;
            if (appBlock) {
              appBlock.TemporaryToken = tempToken.tokenValue;
              appBlock.TemporaryTokenValidity = String(
                Math.floor((tempToken.expiresAt.getTime() - Date.now()) / 1000),
              );
            }
          }
        }

        return reply
          .code(HTTP_STATUS.OK)
          .type(formatted.contentType)
          .send(formatted.body);
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
        operation?: string;
        operation_type?: string;
      };

      if (!query.token) {
        return reply.code(HTTP_STATUS.UNAUTHORIZED).send({
          error: 'Unauthorized',
          message: 'Token is required',
          statusCode: HTTP_STATUS.UNAUTHORIZED,
        });
      }

      const odsaCtxGet = query.operation
        ? { operation: query.operation, operationType: query.operation_type ? parseInt(query.operation_type, 10) : undefined }
        : undefined;

      // Try re-auth state first (read-only, no counter change)
      const reauthStateGet = await getReauthState(query.token);
      if (reauthStateGet) {
        const formatted = await buildEntitlementResponse(
          query.token,
          reauthStateGet.subscriberId,
          query.app,
          query.accept_content_type,
          odsaCtxGet,
        );

        return reply
          .code(HTTP_STATUS.OK)
          .type(formatted.contentType)
          .send(formatted.body);
      }

      // Fall back to ODSA temporary token
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
        odsaCtxGet,
      );

      return reply
        .code(HTTP_STATUS.OK)
        .type(formatted.contentType)
        .send(formatted.body);
    },
  );
}
