/**
 * Private User Identity Handler (ap2013)
 *
 * Returns pseudonym or encrypted identity info when enabled.
 */

import type { ApplicationConfig } from '../protocol/responseTypes.js';
import { EntitlementStatus } from '../protocol/statusCodes.js';

export interface PrivateIdentityConfigData {
  pseudonym?: string;
  identityType?: 'PSEUDONYM' | 'OTHER';
}

export function buildPrivateIdentityConfig(
  status: number,
  provStatus: number,
  tcStatus: number,
  configData?: unknown,
): ApplicationConfig {
  const data = (configData ?? {}) as PrivateIdentityConfigData;

  const result: ApplicationConfig = {
    appId: 'ap2013',
    entitlementStatus: status,
    provStatus,
    tcStatus,
  };

  if (status === EntitlementStatus.ENABLED) {
    const extra: Record<string, string> = {};
    if (data.pseudonym) extra.Pseudonym = data.pseudonym;
    extra.IdentityType = data.identityType ?? 'PSEUDONYM';
    if (Object.keys(extra).length > 0) {
      result.extraParams = extra;
    }
  }

  return result;
}
