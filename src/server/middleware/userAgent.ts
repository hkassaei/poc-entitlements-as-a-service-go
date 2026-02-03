import type { FastifyRequest, FastifyReply } from 'fastify';
import { logger } from '../../config/logger.js';

export interface ParsedUserAgent {
  version: string;
  vendor: string;
  model: string;
  clientType: string;
  os: string;
}

/**
 * Parse TS.43 User-Agent header format:
 * PRD-TS43/<version> (<vendor>; <model>; <client_type>; <OS>)
 */
const UA_PATTERN = /^PRD-TS43\/(\S+)\s+\(([^;]+);\s*([^;]+);\s*([^;]+);\s*([^)]+)\)$/;

export async function userAgentParser(
  request: FastifyRequest,
  _reply: FastifyReply,
): Promise<void> {
  const ua = request.headers['user-agent'];
  if (!ua) {
    logger.debug('No User-Agent header present');
    return;
  }

  const match = ua.match(UA_PATTERN);
  if (!match) {
    logger.debug({ userAgent: ua }, 'User-Agent does not match TS.43 format');
    return;
  }

  const parsed: ParsedUserAgent = {
    version: match[1],
    vendor: match[2].trim(),
    model: match[3].trim(),
    clientType: match[4].trim(),
    os: match[5].trim(),
  };

  (request as any).ts43UserAgent = parsed;
}
