/**
 * Device/User Info Handler (ap2014)
 *
 * Operation-aware service for retrieving subscriber information.
 * Supports GetPhoneNumber and GetSubscriberInfo operations.
 */

import type { ApplicationConfig } from '../protocol/responseTypes.js';
import { SubscriptionResult } from '../protocol/statusCodes.js';
import type { OdsaContext } from './odsaCommon.js';
import { buildOdsaBaseConfig } from './odsaCommon.js';

export interface DeviceUserInfoConfigData {
  msisdn?: string;
  displayName?: string;
  homeCarrier?: string;
}

export function buildDeviceUserInfoConfig(
  status: number,
  provStatus: number,
  tcStatus: number,
  configData?: unknown,
  odsaContext?: OdsaContext,
): ApplicationConfig {
  const data = (configData ?? {}) as DeviceUserInfoConfigData;
  const operation = odsaContext?.operation;

  if (!operation) {
    return {
      appId: 'ap2014',
      entitlementStatus: status,
      provStatus,
      tcStatus,
    };
  }

  switch (operation) {
    case 'GetPhoneNumber':
      return handleGetPhoneNumber(status, provStatus, tcStatus, data);
    case 'GetSubscriberInfo':
      return handleGetSubscriberInfo(status, provStatus, tcStatus, data);
    default:
      return {
        appId: 'ap2014',
        entitlementStatus: status,
        provStatus,
        tcStatus,
      };
  }
}

function handleGetPhoneNumber(
  status: number, provStatus: number, tcStatus: number, data: DeviceUserInfoConfigData,
): ApplicationConfig {
  const extra: Record<string, string> = {};
  if (data.msisdn) extra.MSISDN = data.msisdn;
  return buildOdsaBaseConfig('ap2014', status, provStatus, tcStatus, SubscriptionResult.DONE, extra);
}

function handleGetSubscriberInfo(
  status: number, provStatus: number, tcStatus: number, data: DeviceUserInfoConfigData,
): ApplicationConfig {
  const extra: Record<string, string> = {};
  if (data.msisdn) extra.MSISDN = data.msisdn;
  if (data.displayName) extra.DisplayName = data.displayName;
  if (data.homeCarrier) extra.HomeCarrier = data.homeCarrier;
  return buildOdsaBaseConfig('ap2014', status, provStatus, tcStatus, SubscriptionResult.DONE, extra);
}
