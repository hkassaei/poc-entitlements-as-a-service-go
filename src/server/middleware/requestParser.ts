import type { FastifyRequest, FastifyReply } from 'fastify';

/**
 * For GET requests, normalize query string params into the same shape
 * as a POST body so downstream handlers can work with a uniform interface.
 * POST bodies are validated natively by Fastify's schema validation.
 */
export async function requestParser(
  request: FastifyRequest,
  _reply: FastifyReply,
): Promise<void> {
  if (request.method === 'GET' && request.query) {
    // Attach normalized query as parsedRequest for uniform access
    (request as any).parsedRequest = request.query;
  } else if (request.method === 'POST' && request.body) {
    (request as any).parsedRequest = request.body;
  }
}
