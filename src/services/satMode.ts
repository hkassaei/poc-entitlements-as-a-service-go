/**
 * Satellite Mode Handler (ap2016)
 *
 * Returns PLMN allow/barred lists and service constraints
 * for satellite connectivity mode when enabled.
 */

import type { ApplicationConfig } from '../protocol/responseTypes.js';
import { EntitlementStatus } from '../protocol/statusCodes.js';

export interface SatModeConfigData {
  plmnAllow?: string[];
  plmnBarred?: string[];
  serviceConstraints?: string;
}

export function buildSatModeConfig(
  status: number,
  provStatus: number,
  tcStatus: number,
  configData?: unknown,
): ApplicationConfig {
  const data = (configData ?? {}) as SatModeConfigData;

  const result: ApplicationConfig = {
    appId: 'ap2016',
    entitlementStatus: status,
    provStatus,
    tcStatus,
  };

  if (status === EntitlementStatus.ENABLED) {
    const extra: Record<string, string> = {};
    if (data.plmnAllow && data.plmnAllow.length > 0) extra.PLMNAllow = data.plmnAllow.join(',');
    if (data.plmnBarred && data.plmnBarred.length > 0) extra.PLMNBarred = data.plmnBarred.join(',');
    if (data.serviceConstraints) extra.ServiceConstraints = data.serviceConstraints;
    if (Object.keys(extra).length > 0) {
      result.extraParams = extra;
    }
  }

  return result;
}
