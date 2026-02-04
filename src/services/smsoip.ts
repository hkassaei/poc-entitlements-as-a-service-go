/**
 * SMSoIP (ap2005) Service Handler
 *
 * Builds the ApplicationConfig for SMS over IP entitlements.
 */

import type { ApplicationConfig } from '../protocol/responseTypes.js';
import { EntitlementStatus } from '../protocol/statusCodes.js';

interface SmsOipConfigData {
  addresses?: Array<{ addrType: string; addr: string }>;
}

const DEFAULT_ADDRESSES = [
  { addrType: '1', addr: 'smsc.operator.com' },
];

export function buildSmsOipConfig(
  status: number,
  provStatus: number,
  tcStatus: number,
  configData?: unknown,
): ApplicationConfig {
  const config = (configData ?? {}) as SmsOipConfigData;

  const result: ApplicationConfig = {
    appId: 'ap2005',
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

  return result;
}
