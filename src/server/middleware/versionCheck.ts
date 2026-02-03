import type { FastifyRequest, FastifyReply } from 'fastify';
import { config } from '../../config/index.js';
import { HTTP_STATUS } from '../../config/constants.js';

/**
 * Check entitlement_version against SUPPORTED_VERSIONS.
 * Reply 406 Not Acceptable if the version is unsupported.
 */
export async function versionCheck(
  request: FastifyRequest,
  reply: FastifyReply,
): Promise<void> {
  const parsedRequest = (request as any).parsedRequest;
  if (!parsedRequest?.entitlement_version) {
    return; // Let schema validation handle missing fields
  }

  const version = parsedRequest.entitlement_version;
  if (!config.supportedVersions.includes(version)) {
    reply.code(HTTP_STATUS.NOT_ACCEPTABLE).send({
      error: 'Not Acceptable',
      message: `Unsupported entitlement_version: ${version}. Supported: ${config.supportedVersions.join(', ')}`,
    });
  }
}
