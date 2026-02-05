/**
 * Audit Logger Middleware
 *
 * Fastify onResponse hook that logs all requests to the audit_log table.
 * Extracts subscriber context from request state when available.
 */

import type { FastifyRequest, FastifyReply } from 'fastify';
import { logAuditEvent, type AuditEvent } from '../../db/auditService.js';

/**
 * Fastify onResponse hook for audit logging.
 * Fires after the response has been sent, so it doesn't add latency.
 */
export async function auditLogger(
  request: FastifyRequest,
  reply: FastifyReply,
): Promise<void> {
  // Extract request body/query safely
  const body = request.body as Record<string, unknown> | undefined;
  const query = request.query as Record<string, unknown> | undefined;

  // Extract subscriber context if available (set by auth handlers)
  // This is stored in request context by the entitlement routes
  const subscriberContext = (request as FastifyRequest & {
    subscriberContext?: { subscriberId?: string };
  }).subscriberContext;

  const event: AuditEvent = {
    subscriberId: subscriberContext?.subscriberId,
    appId: body?.app as string | undefined,
    operation: body?.operation as string | undefined,
    requestBody: body,
    requestQuery: query,
    responseCode: reply.statusCode,
    clientIp: request.ip,
    userAgent: request.headers['user-agent'] || undefined,
    method: request.method,
    path: request.url.split('?')[0], // Strip query string
  };

  // Fire and forget — don't await, don't block the response
  logAuditEvent(event).catch(() => {
    // Error already logged in logAuditEvent
  });
}
