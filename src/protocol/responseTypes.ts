/**
 * GSMA TS.43 Entitlement Response Types
 *
 * Internal representation of entitlement responses. These interfaces are
 * consumed by both the JSON and XML builders to produce wire-format responses.
 */

export interface AddressConfig {
  addrType: string; // "1" = FQDN, "2" = IPv4, "3" = IPv6
  addr: string;
}

export interface ApplicationConfig {
  appId: string;
  entitlementStatus: number;
  addrStatus?: number;
  tcStatus?: number;
  provStatus?: number;
  serviceFlowUrl?: string;
  addresses?: AddressConfig[];
  extraParams?: Record<string, string>;
}

export interface ServiceEntitlementResponse {
  version: string;
  validity: number; // TTL in seconds
  token: string;
  applications: ApplicationConfig[];
}
