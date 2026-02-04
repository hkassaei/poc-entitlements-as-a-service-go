/**
 * Entitlement Response Builder
 *
 * Orchestrates per-service handlers and format builders to produce
 * a complete TS.43 entitlement response in JSON or XML.
 */

import { eq } from 'drizzle-orm';
import { db } from '../db/index.js';
import { entitlements } from '../db/schema.js';
import { config } from '../config/index.js';
import type { ServiceEntitlementResponse, ApplicationConfig } from './responseTypes.js';
import { buildJsonResponse } from './jsonBuilder.js';
import { buildXmlResponse } from './xmlBuilder.js';
import { buildVoWiFiConfig } from '../services/vowifi.js';
import { buildVoLTEConfig } from '../services/volte.js';
import { buildSmsOipConfig } from '../services/smsoip.js';
import { buildCompanionConfig } from '../services/odsaCompanion.js';
import { buildPrimaryConfig } from '../services/odsaPrimary.js';
import { buildDataPlanConfig } from '../services/dataPlan.js';
import { buildServerOdsaConfig } from '../services/serverOdsa.js';
import { buildDcbConfig } from '../services/directCarrierBilling.js';
import { buildPrivateIdentityConfig } from '../services/privateUserIdentity.js';
import { buildDeviceUserInfoConfig } from '../services/deviceUserInfo.js';
import { buildAppAuthConfig } from '../services/appAuthentication.js';
import { buildSatModeConfig } from '../services/satMode.js';
import type { OdsaContext } from '../services/odsaCommon.js';

export interface FormattedResponse {
  body: object | string;
  contentType: string;
}

/**
 * Build a complete entitlement response for a subscriber and app.
 *
 * Looks up the entitlement from the database, routes to the appropriate
 * service handler, and formats the response as JSON or XML.
 */
export async function buildEntitlementResponse(
  token: string,
  subscriberId: string,
  appId: string,
  acceptContentType?: string,
  odsaContext?: OdsaContext,
): Promise<FormattedResponse> {
  // Look up entitlement for this subscriber + app
  const rows = await db
    .select()
    .from(entitlements)
    .where(eq(entitlements.subscriberId, subscriberId))
    .limit(20);

  const entitlement = rows.find((e) => e.appId === appId);

  // Build per-app config via the appropriate service handler
  const appConfig = buildAppConfig(
    appId,
    entitlement?.status ?? 1,
    entitlement?.provStatus ?? 0,
    entitlement?.tcStatus ?? 0,
    entitlement?.configData,
    odsaContext,
  );

  const response: ServiceEntitlementResponse = {
    version: '1',
    validity: config.defaultConfigValidity,
    token,
    applications: [appConfig],
  };

  if (acceptContentType === 'xml') {
    return {
      body: buildXmlResponse(response),
      contentType: 'application/xml',
    };
  }

  return {
    body: buildJsonResponse(response),
    contentType: 'application/json',
  };
}

/**
 * Route to the correct service handler based on appId.
 * Unsupported apps get a generic config with just the status fields.
 */
function buildAppConfig(
  appId: string,
  status: number,
  provStatus: number,
  tcStatus: number,
  configData?: unknown,
  odsaContext?: OdsaContext,
): ApplicationConfig {
  switch (appId) {
    case 'ap2004':
      return buildVoWiFiConfig(status, provStatus, tcStatus, configData);
    case 'ap2003':
      return buildVoLTEConfig(status, provStatus, tcStatus, configData);
    case 'ap2005':
      return buildSmsOipConfig(status, provStatus, tcStatus, configData);
    case 'ap2006':
      return buildCompanionConfig(status, provStatus, tcStatus, configData, odsaContext);
    case 'ap2009':
      return buildPrimaryConfig(status, provStatus, tcStatus, configData, odsaContext);
    case 'ap2010':
      return buildDataPlanConfig(status, provStatus, tcStatus, configData, odsaContext);
    case 'ap2011':
      return buildServerOdsaConfig(status, provStatus, tcStatus, configData, odsaContext);
    case 'ap2012':
      return buildDcbConfig(status, provStatus, tcStatus, configData);
    case 'ap2013':
      return buildPrivateIdentityConfig(status, provStatus, tcStatus, configData);
    case 'ap2014':
      return buildDeviceUserInfoConfig(status, provStatus, tcStatus, configData, odsaContext);
    case 'ap2015':
      return buildAppAuthConfig(status, provStatus, tcStatus, configData);
    case 'ap2016':
      return buildSatModeConfig(status, provStatus, tcStatus, configData);
    default:
      // Generic handler for unsupported app IDs
      return {
        appId,
        entitlementStatus: status,
        provStatus,
        tcStatus,
      };
  }
}
