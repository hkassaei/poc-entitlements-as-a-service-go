/**
 * TS.43 JSON Response Builder
 *
 * Converts the internal ServiceEntitlementResponse into the GSMA TS.43
 * JSON format with Vers, Token, and per-application blocks.
 */

import type { ServiceEntitlementResponse, ApplicationConfig } from './responseTypes.js';

/**
 * Build a TS.43 JSON response object.
 *
 * Output format:
 * {
 *   "Vers": { "version": "1", "validity": "172800" },
 *   "Token": { "token": "..." },
 *   "ap2004": { "EntitlementStatus": "1", ... }
 * }
 */
export function buildJsonResponse(response: ServiceEntitlementResponse): object {
  const result: Record<string, unknown> = {
    Vers: {
      version: response.version,
      validity: String(response.validity),
    },
    Token: {
      token: response.token,
    },
  };

  for (const app of response.applications) {
    result[app.appId] = buildApplicationBlock(app);
  }

  return result;
}

function buildApplicationBlock(app: ApplicationConfig): Record<string, unknown> {
  const block: Record<string, unknown> = {
    EntitlementStatus: String(app.entitlementStatus),
  };

  if (app.addrStatus !== undefined) {
    block.AddrStatus = String(app.addrStatus);
  }

  if (app.tcStatus !== undefined) {
    block.TC_Status = String(app.tcStatus);
  }

  if (app.provStatus !== undefined) {
    block.ProvStatus = String(app.provStatus);
  }

  if (app.serviceFlowUrl) {
    block.ServiceFlow_URL = app.serviceFlowUrl;
  }

  if (app.addresses && app.addresses.length > 0) {
    const addrBlock: Record<string, { AddrType: string; Addr: string }> = {};
    for (let i = 0; i < app.addresses.length; i++) {
      const addr = app.addresses[i]!;
      addrBlock[String(i + 1)] = {
        AddrType: addr.addrType,
        Addr: addr.addr,
      };
    }
    block.Addr = addrBlock;
  }

  if (app.extraParams) {
    for (const [key, value] of Object.entries(app.extraParams)) {
      block[key] = value;
    }
  }

  return block;
}
