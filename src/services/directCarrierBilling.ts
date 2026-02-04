/**
 * Direct Carrier Billing Handler (ap2012)
 *
 * Simple builder (no operations) for carrier billing entitlement.
 * Returns entitlement status and T&C status; when T&C acceptance
 * is required, provides a service flow URL for the billing portal.
 */

import type { ApplicationConfig } from '../protocol/responseTypes.js';
import { EntitlementStatus, TcStatus } from '../protocol/statusCodes.js';

export interface DcbConfigData {
  serviceFlowUrl?: string;
}

export function buildDcbConfig(
  status: number,
  provStatus: number,
  tcStatus: number,
  configData?: unknown,
): ApplicationConfig {
  const data = (configData ?? {}) as DcbConfigData;

  const result: ApplicationConfig = {
    appId: 'ap2012',
    entitlementStatus: status,
    provStatus,
    tcStatus,
  };

  if (status === EntitlementStatus.INCOMPATIBLE) {
    result.extraParams = { Message: 'Direct Carrier Billing is not available for this device.' };
    return result;
  }

  if (status === EntitlementStatus.DISABLED && tcStatus === TcStatus.REQUIRES_ACCEPTANCE && data.serviceFlowUrl) {
    result.serviceFlowUrl = data.serviceFlowUrl;
  }

  return result;
}
