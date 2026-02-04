/**
 * VoWiFi (ap2004) Service Handler
 *
 * Builds the ApplicationConfig for Wi-Fi Calling entitlements.
 */

import type { ApplicationConfig } from '../protocol/responseTypes.js';
import { EntitlementStatus, TcStatus } from '../protocol/statusCodes.js';

interface VoWiFiConfigData {
  addresses?: Array<{ addrType: string; addr: string }>;
  serviceFlowUrl?: string;
}

const DEFAULT_ADDRESSES = [
  { addrType: '1', addr: 'epdg.operator.com' },
  { addrType: '1', addr: 'pcscf.operator.com' },
];

export function buildVoWiFiConfig(
  status: number,
  provStatus: number,
  tcStatus: number,
  configData?: unknown,
): ApplicationConfig {
  const config = (configData ?? {}) as VoWiFiConfigData;

  const result: ApplicationConfig = {
    appId: 'ap2004',
    entitlementStatus: status,
    tcStatus,
    provStatus,
  };

  if (status === EntitlementStatus.ENABLED) {
    result.addrStatus = 1;
    result.addresses = config.addresses ?? DEFAULT_ADDRESSES;
  } else {
    result.addrStatus = 0;
  }

  if (tcStatus === TcStatus.REQUIRES_ACCEPTANCE && config.serviceFlowUrl) {
    result.serviceFlowUrl = config.serviceFlowUrl;
  }

  return result;
}
