/**
 * App Authentication Handler (ap2015)
 *
 * Returns operator token endpoint info when enabled,
 * allowing apps to obtain carrier-scoped auth tokens.
 */

import type { ApplicationConfig } from '../protocol/responseTypes.js';
import { EntitlementStatus } from '../protocol/statusCodes.js';

export interface AppAuthConfigData {
  operatorTokenUrl?: string;
  appTokenScope?: string;
}

export function buildAppAuthConfig(
  status: number,
  provStatus: number,
  tcStatus: number,
  configData?: unknown,
): ApplicationConfig {
  const data = (configData ?? {}) as AppAuthConfigData;

  const result: ApplicationConfig = {
    appId: 'ap2015',
    entitlementStatus: status,
    provStatus,
    tcStatus,
  };

  if (status === EntitlementStatus.ENABLED) {
    const extra: Record<string, string> = {};
    if (data.operatorTokenUrl) extra.OperatorTokenUrl = data.operatorTokenUrl;
    if (data.appTokenScope) extra.AppTokenScope = data.appTokenScope;
    if (Object.keys(extra).length > 0) {
      result.extraParams = extra;
    }
  }

  return result;
}
