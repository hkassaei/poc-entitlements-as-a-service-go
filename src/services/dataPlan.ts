/**
 * Data Plan Information Handler (ap2010)
 *
 * Operation-aware service for data plan queries and acquisition.
 * Supports CheckEligibility, AcquirePlan, and GetPlanDetails operations.
 */

import type { ApplicationConfig } from '../protocol/responseTypes.js';
import { EntitlementStatus, SubscriptionResult } from '../protocol/statusCodes.js';
import type { OdsaContext } from './odsaCommon.js';
import { buildOdsaBaseConfig } from './odsaCommon.js';

export interface DataPlanConfigData {
  planName?: string;
  planId?: string;
  dataAllowanceBytes?: number;
  dataUsedBytes?: number;
  billingCycleEnd?: string;
  accessType?: string;
  dataType?: 'metered' | 'unmetered';
  boostEligible?: boolean;
  serviceFlowUrl?: string;
}

export function buildDataPlanConfig(
  status: number,
  provStatus: number,
  tcStatus: number,
  configData?: unknown,
  odsaContext?: OdsaContext,
): ApplicationConfig {
  const data = (configData ?? {}) as DataPlanConfigData;
  const operation = odsaContext?.operation;

  if (!operation) {
    return {
      appId: 'ap2010',
      entitlementStatus: status,
      provStatus,
      tcStatus,
    };
  }

  switch (operation) {
    case 'CheckEligibility':
      return handleCheckEligibility(status, provStatus, tcStatus, data);
    case 'AcquirePlan':
      return handleAcquirePlan(status, provStatus, tcStatus, data);
    case 'GetPlanDetails':
      return handleGetPlanDetails(status, provStatus, tcStatus, data);
    default:
      return {
        appId: 'ap2010',
        entitlementStatus: status,
        provStatus,
        tcStatus,
      };
  }
}

function handleCheckEligibility(
  status: number, provStatus: number, tcStatus: number, data: DataPlanConfigData,
): ApplicationConfig {
  const isEligible = data.boostEligible === true || data.planId !== undefined;
  const entitlementStatus = isEligible ? EntitlementStatus.ENABLED : EntitlementStatus.DISABLED;
  return buildOdsaBaseConfig('ap2010', entitlementStatus, provStatus, tcStatus, SubscriptionResult.DONE);
}

function handleAcquirePlan(
  status: number, provStatus: number, tcStatus: number, data: DataPlanConfigData,
): ApplicationConfig {
  if (data.serviceFlowUrl) {
    const extra: Record<string, string> = {
      ServiceFlow_URL: data.serviceFlowUrl,
    };
    if (data.planId) extra.PlanId = data.planId;
    if (data.planName) extra.PlanName = data.planName;
    return buildOdsaBaseConfig('ap2010', status, provStatus, tcStatus, SubscriptionResult.CONTINUE_TO_WS, extra);
  }

  const extra: Record<string, string> = {};
  if (data.planId) extra.PlanId = data.planId;
  if (data.planName) extra.PlanName = data.planName;
  return buildOdsaBaseConfig('ap2010', status, provStatus, tcStatus, SubscriptionResult.DONE, extra);
}

function handleGetPlanDetails(
  status: number, provStatus: number, tcStatus: number, data: DataPlanConfigData,
): ApplicationConfig {
  const extra: Record<string, string> = {};
  if (data.planId) extra.PlanId = data.planId;
  if (data.planName) extra.PlanName = data.planName;
  if (data.dataAllowanceBytes !== undefined) extra.DataAllowanceBytes = String(data.dataAllowanceBytes);
  if (data.dataUsedBytes !== undefined) extra.DataUsedBytes = String(data.dataUsedBytes);
  if (data.billingCycleEnd) extra.BillingCycleEnd = data.billingCycleEnd;
  if (data.accessType) extra.AccessType = data.accessType;
  if (data.dataType) extra.DataType = data.dataType;
  if (data.boostEligible !== undefined) extra.BoostEligible = String(data.boostEligible);

  return buildOdsaBaseConfig('ap2010', status, provStatus, tcStatus, SubscriptionResult.DONE, extra);
}
