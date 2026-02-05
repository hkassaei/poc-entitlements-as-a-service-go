/**
 * Audit Service
 *
 * Provides centralized audit logging with proper redaction of sensitive data.
 * Uses an allowlist approach — only explicitly listed fields are logged.
 *
 * Sensitive fields (eap_relay, token, IMSI, etc.) are either:
 * - Completely omitted
 * - Redacted to show only presence/type (e.g., "token": "[REDACTED:64chars]")
 */

import { db } from './index.js';
import { auditLog } from './schema.js';
import { logger } from '../config/logger.js';

/**
 * Fields that are safe to log in full.
 * Anything not in this list is either redacted or omitted.
 */
const ALLOWED_FIELDS = new Set([
  'app',
  'terminal_id',
  'entitlement_version',
  'accept_content_type',
  'operation',
  'operation_type',
]);

/**
 * Fields that should be redacted (show presence but not value).
 * Format: "[REDACTED:Nchars]" or "[REDACTED:present]"
 */
const REDACTED_FIELDS = new Set([
  'imsi',
  'token',
  'eap_relay',
]);

/**
 * Fields that should be completely omitted from logs.
 * These are either too large or too sensitive even for redacted form.
 */
const OMITTED_FIELDS = new Set([
  'ki',
  'op',
  'password',
  'secret',
]);

export interface AuditEvent {
  subscriberId?: string;
  appId?: string;
  operation?: string;
  requestBody?: Record<string, unknown>;
  requestQuery?: Record<string, unknown>;
  responseCode: number;
  clientIp?: string;
  userAgent?: string;
  method: string;
  path: string;
}

/**
 * Redact a single value based on its type and content.
 */
function redactValue(value: unknown): string {
  if (value === null || value === undefined) {
    return '[REDACTED:null]';
  }
  if (typeof value === 'string') {
    if (value.length > 100) {
      // Likely base64 blob (eap_relay, etc.)
      return `[REDACTED:${value.length}chars]`;
    }
    // Shorter strings get length indication
    return `[REDACTED:${value.length}chars]`;
  }
  if (typeof value === 'number') {
    return '[REDACTED:number]';
  }
  if (Array.isArray(value)) {
    return `[REDACTED:array:${value.length}items]`;
  }
  if (typeof value === 'object') {
    return '[REDACTED:object]';
  }
  return '[REDACTED:present]';
}

/**
 * Extract and redact request data using allowlist approach.
 * Only fields in ALLOWED_FIELDS are logged in full.
 * Fields in REDACTED_FIELDS show presence but not value.
 * Fields in OMITTED_FIELDS are completely removed.
 * Unknown fields are omitted for safety.
 */
export function extractAuditableRequest(
  body?: Record<string, unknown>,
  query?: Record<string, unknown>,
): Record<string, unknown> {
  const result: Record<string, unknown> = {};

  const processFields = (source: Record<string, unknown> | undefined, prefix: string) => {
    if (!source) return;

    for (const [key, value] of Object.entries(source)) {
      const fieldName = key.toLowerCase();

      // Skip completely omitted fields
      if (OMITTED_FIELDS.has(fieldName)) {
        continue;
      }

      // Redact sensitive fields
      if (REDACTED_FIELDS.has(fieldName)) {
        result[`${prefix}${key}`] = redactValue(value);
        continue;
      }

      // Allow safe fields in full
      if (ALLOWED_FIELDS.has(fieldName)) {
        result[`${prefix}${key}`] = value;
        continue;
      }

      // Unknown fields are omitted (fail-safe)
      // This prevents accidentally logging new sensitive fields
    }
  };

  processFields(body, '');
  processFields(query, 'query_');

  return result;
}

/**
 * Log an audit event to the database.
 * This is fire-and-forget — failures are logged but don't affect the request.
 */
export async function logAuditEvent(event: AuditEvent): Promise<void> {
  // Skip health check endpoints
  if (event.path === '/health' || event.path === '/ready') {
    return;
  }

  try {
    const requestSummary = extractAuditableRequest(event.requestBody, event.requestQuery);

    // Add request metadata
    requestSummary._method = event.method;
    requestSummary._path = event.path;

    await db.insert(auditLog).values({
      subscriberId: event.subscriberId || null,
      appId: event.appId || null,
      operation: event.operation || null,
      requestSummary,
      responseCode: event.responseCode,
      clientIp: event.clientIp || null,
      userAgent: event.userAgent || null,
    });
  } catch (err) {
    // Log the error but don't fail the request
    logger.error({ err, path: event.path }, 'Failed to write audit log');
  }
}

/**
 * Batch insert audit events (for high-throughput scenarios).
 * Not currently used, but available for future optimization.
 */
export async function logAuditEventsBatch(events: AuditEvent[]): Promise<void> {
  if (events.length === 0) return;

  try {
    const values = events
      .filter((e) => e.path !== '/health' && e.path !== '/ready')
      .map((event) => {
        const requestSummary = extractAuditableRequest(event.requestBody, event.requestQuery);
        requestSummary._method = event.method;
        requestSummary._path = event.path;

        return {
          subscriberId: event.subscriberId || null,
          appId: event.appId || null,
          operation: event.operation || null,
          requestSummary,
          responseCode: event.responseCode,
          clientIp: event.clientIp || null,
          userAgent: event.userAgent || null,
        };
      });

    if (values.length > 0) {
      await db.insert(auditLog).values(values);
    }
  } catch (err) {
    logger.error({ err, count: events.length }, 'Failed to write audit log batch');
  }
}
