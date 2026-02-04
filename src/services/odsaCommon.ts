/**
 * ODSA Common Types and Helpers
 *
 * Shared interface for configData JSONB used by ap2006/ap2009
 * and helper to build ODSA response configs.
 */

import type { ApplicationConfig } from '../protocol/responseTypes.js';

export interface OdsaConfigData {
  subscriptionState?: 'eligible' | 'pending' | 'active' | 'suspended';
  smdpAddress?: string;
  profileIccid?: string;
  profileType?: string;
  companionDeviceImei?: string;
  primaryDeviceImsi?: string;
  planId?: string;
  planName?: string;
  serviceFlowUrl?: string;
  serviceFlowUserData?: string;
}

export interface OdsaContext {
  operation?: string;
  operationType?: number;
}

/**
 * Build a base ODSA ApplicationConfig with SubscriptionResult in extraParams.
 */
export function buildOdsaBaseConfig(
  appId: string,
  status: number,
  provStatus: number,
  tcStatus: number,
  subscriptionResult: number,
  extraFields?: Record<string, string>,
): ApplicationConfig {
  const extraParams: Record<string, string> = {
    SubscriptionResult: String(subscriptionResult),
    ...extraFields,
  };

  return {
    appId,
    entitlementStatus: status,
    provStatus,
    tcStatus,
    extraParams,
  };
}
