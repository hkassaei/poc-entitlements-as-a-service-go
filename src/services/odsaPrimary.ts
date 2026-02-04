/**
 * ODSA Primary Device Handler (ap2009)
 *
 * Handles eSIM lifecycle operations for primary devices.
 * Same as companion plus AcquirePlan.
 */

import type { ApplicationConfig } from '../protocol/responseTypes.js';
import { EntitlementStatus, SubscriptionResult } from '../protocol/statusCodes.js';
import { getActivationCode } from './mockSmdp.js';
import type { OdsaConfigData, OdsaContext } from './odsaCommon.js';
import { buildOdsaBaseConfig } from './odsaCommon.js';

export function buildPrimaryConfig(
  status: number,
  provStatus: number,
  tcStatus: number,
  configData?: unknown,
  odsaContext?: OdsaContext,
): ApplicationConfig {
  const data = (configData ?? {}) as OdsaConfigData;
  const operation = odsaContext?.operation;

  if (!operation) {
    return {
      appId: 'ap2009',
      entitlementStatus: status,
      provStatus,
      tcStatus,
    };
  }

  switch (operation) {
    case 'CheckEligibility':
      return handleCheckEligibility(status, provStatus, tcStatus, data);
    case 'ManageSubscription':
      return handleManageSubscription(status, provStatus, tcStatus, data);
    case 'ManageService':
      return handleManageService(status, provStatus, tcStatus, data);
    case 'AcquireConfiguration':
      return handleAcquireConfiguration(status, provStatus, tcStatus, data);
    case 'AcquirePlan':
      return handleAcquirePlan(status, provStatus, tcStatus, data);
    case 'AcquireTemporaryToken':
      return buildOdsaBaseConfig('ap2009', status, provStatus, tcStatus, SubscriptionResult.DONE);
    default:
      return {
        appId: 'ap2009',
        entitlementStatus: status,
        provStatus,
        tcStatus,
      };
  }
}

function handleCheckEligibility(
  status: number, provStatus: number, tcStatus: number, data: OdsaConfigData,
): ApplicationConfig {
  const isEligible = data.subscriptionState === 'eligible' || data.subscriptionState === 'active';
  const entitlementStatus = isEligible ? EntitlementStatus.ENABLED : EntitlementStatus.DISABLED;
  return buildOdsaBaseConfig('ap2009', entitlementStatus, provStatus, tcStatus, SubscriptionResult.DONE);
}

function handleManageSubscription(
  status: number, provStatus: number, tcStatus: number, data: OdsaConfigData,
): ApplicationConfig {
  if (data.subscriptionState === 'eligible' && data.serviceFlowUrl) {
    const config = buildOdsaBaseConfig(
      'ap2009', status, provStatus, tcStatus, SubscriptionResult.CONTINUE_TO_WS,
      { ServiceFlow_URL: data.serviceFlowUrl },
    );
    if (data.serviceFlowUserData) {
      config.extraParams!.ServiceFlow_UserData = data.serviceFlowUserData;
    }
    return config;
  }

  if (data.subscriptionState === 'active' && data.smdpAddress) {
    const profileKey = data.profileType ?? 'default';
    const activation = getActivationCode(profileKey);
    return buildOdsaBaseConfig(
      'ap2009', status, provStatus, tcStatus, SubscriptionResult.DOWNLOAD_PROFILE,
      {
        'SMDP+Address': data.smdpAddress,
        'SMDP+ActivationCode': activation.activationCode,
        ProfileICCID: activation.iccid,
      },
    );
  }

  return buildOdsaBaseConfig('ap2009', status, provStatus, tcStatus, SubscriptionResult.DONE);
}

function handleManageService(
  status: number, provStatus: number, tcStatus: number, _data: OdsaConfigData,
): ApplicationConfig {
  return buildOdsaBaseConfig('ap2009', status, provStatus, tcStatus, SubscriptionResult.DONE);
}

function handleAcquireConfiguration(
  status: number, provStatus: number, tcStatus: number, data: OdsaConfigData,
): ApplicationConfig {
  const extra: Record<string, string> = {};
  if (data.smdpAddress) {
    extra['SMDP+Address'] = data.smdpAddress;
  }
  if (data.profileIccid) {
    extra.ProfileICCID = data.profileIccid;
  }
  return buildOdsaBaseConfig('ap2009', status, provStatus, tcStatus, SubscriptionResult.DONE, extra);
}

function handleAcquirePlan(
  status: number, provStatus: number, tcStatus: number, data: OdsaConfigData,
): ApplicationConfig {
  if (data.serviceFlowUrl) {
    const extra: Record<string, string> = { ServiceFlow_URL: data.serviceFlowUrl };
    if (data.planId) extra.PlanId = data.planId;
    if (data.planName) extra.PlanName = data.planName;
    return buildOdsaBaseConfig('ap2009', status, provStatus, tcStatus, SubscriptionResult.CONTINUE_TO_WS, extra);
  }

  const extra: Record<string, string> = {};
  if (data.planId) extra.PlanId = data.planId;
  if (data.planName) extra.PlanName = data.planName;
  return buildOdsaBaseConfig('ap2009', status, provStatus, tcStatus, SubscriptionResult.DONE, extra);
}
