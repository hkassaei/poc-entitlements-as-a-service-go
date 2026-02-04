/**
 * Server-Initiated ODSA Handler (ap2011)
 *
 * Handles server-initiated eSIM lifecycle operations,
 * typically scoped to enterprise/managed device contexts.
 */

import type { ApplicationConfig } from '../protocol/responseTypes.js';
import { EntitlementStatus, SubscriptionResult } from '../protocol/statusCodes.js';
import { getActivationCode } from './mockSmdp.js';
import type { OdsaContext } from './odsaCommon.js';
import { buildOdsaBaseConfig } from './odsaCommon.js';

export interface ServerOdsaConfigData {
  enterpriseId?: string;
  smdpAddress?: string;
  profileType?: string;
  subscriptionState?: 'eligible' | 'pending' | 'active' | 'suspended';
  serviceFlowUrl?: string;
  serviceFlowUserData?: string;
}

export function buildServerOdsaConfig(
  status: number,
  provStatus: number,
  tcStatus: number,
  configData?: unknown,
  odsaContext?: OdsaContext,
): ApplicationConfig {
  const data = (configData ?? {}) as ServerOdsaConfigData;
  const operation = odsaContext?.operation;

  if (!operation) {
    return {
      appId: 'ap2011',
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
    default:
      return {
        appId: 'ap2011',
        entitlementStatus: status,
        provStatus,
        tcStatus,
      };
  }
}

function handleCheckEligibility(
  status: number, provStatus: number, tcStatus: number, data: ServerOdsaConfigData,
): ApplicationConfig {
  const isEligible = data.subscriptionState === 'eligible' || data.subscriptionState === 'active';
  const entitlementStatus = isEligible ? EntitlementStatus.ENABLED : EntitlementStatus.DISABLED;
  const extra: Record<string, string> = {};
  if (data.enterpriseId) extra.EnterpriseId = data.enterpriseId;
  return buildOdsaBaseConfig('ap2011', entitlementStatus, provStatus, tcStatus, SubscriptionResult.DONE, extra);
}

function handleManageSubscription(
  status: number, provStatus: number, tcStatus: number, data: ServerOdsaConfigData,
): ApplicationConfig {
  if (data.subscriptionState === 'eligible' && data.serviceFlowUrl) {
    const extra: Record<string, string> = { ServiceFlow_URL: data.serviceFlowUrl };
    if (data.serviceFlowUserData) extra.ServiceFlow_UserData = data.serviceFlowUserData;
    if (data.enterpriseId) extra.EnterpriseId = data.enterpriseId;
    return buildOdsaBaseConfig('ap2011', status, provStatus, tcStatus, SubscriptionResult.CONTINUE_TO_WS, extra);
  }

  if (data.subscriptionState === 'active' && data.smdpAddress) {
    const profileKey = data.profileType ?? 'default';
    const activation = getActivationCode(profileKey);
    return buildOdsaBaseConfig('ap2011', status, provStatus, tcStatus, SubscriptionResult.DOWNLOAD_PROFILE, {
      'SMDP+Address': data.smdpAddress,
      'SMDP+ActivationCode': activation.activationCode,
      ProfileICCID: activation.iccid,
    });
  }

  return buildOdsaBaseConfig('ap2011', status, provStatus, tcStatus, SubscriptionResult.DONE);
}

function handleManageService(
  status: number, provStatus: number, tcStatus: number, _data: ServerOdsaConfigData,
): ApplicationConfig {
  return buildOdsaBaseConfig('ap2011', status, provStatus, tcStatus, SubscriptionResult.DONE);
}
