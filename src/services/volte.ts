/**
 * Voice-over-Cellular / VoLTE (ap2003) Service Handler
 *
 * Builds the ApplicationConfig for VoLTE and VoNR entitlements.
 */

import type { ApplicationConfig } from '../protocol/responseTypes.js';
import { EntitlementStatus } from '../protocol/statusCodes.js';

interface VoLTEConfigData {
  addresses?: Array<{ addrType: string; addr: string }>;
  volteEntitled?: string;
  vonrEntitled?: string;
}

const DEFAULT_ADDRESSES = [
  { addrType: '1', addr: 'pcscf.operator.com' },
];

export function buildVoLTEConfig(
  status: number,
  provStatus: number,
  tcStatus: number,
  configData?: unknown,
): ApplicationConfig {
  const config = (configData ?? {}) as VoLTEConfigData;

  const result: ApplicationConfig = {
    appId: 'ap2003',
    entitlementStatus: status,
    tcStatus,
    provStatus,
    extraParams: {},
  };

  if (status === EntitlementStatus.ENABLED) {
    result.addrStatus = 1;
    result.addresses = config.addresses ?? DEFAULT_ADDRESSES;
    result.extraParams!.VoLTE_Entitled = config.volteEntitled ?? '1';
    result.extraParams!.VoNR_Entitled = config.vonrEntitled ?? '1';
  } else {
    result.addrStatus = 0;
    result.extraParams!.VoLTE_Entitled = '0';
    result.extraParams!.VoNR_Entitled = '0';
  }

  return result;
}
