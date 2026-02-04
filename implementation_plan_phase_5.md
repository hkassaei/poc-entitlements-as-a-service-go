# Phase 5: Core Entitlement Services

## Overview

Build the response formatting layer and per-service handlers for VoWiFi (ap2004), VoLTE/VoNR (ap2003), and SMSoIP (ap2005). Replace the current flat JSON response with the structured GSMA TS.43 format supporting both JSON and XML (WAP-Provisioning). Add seed data so test subscribers have real entitlement configurations.

## Implementation Steps

### Step 1: Response types — `src/protocol/responseTypes.ts` (new)

Shared TypeScript interfaces for the internal entitlement response model:

```typescript
interface ServiceEntitlementResponse {
  version: string;
  validity: number;
  token: string;
  applications: ApplicationConfig[];
}

interface ApplicationConfig {
  appId: string;
  entitlementStatus: number;
  addrStatus?: number;
  tcStatus?: number;
  provStatus?: number;
  serviceFlowUrl?: string;
  addresses?: AddressConfig[];
  extraParams?: Record<string, string>;  // VoLTE_Entitled, VoNR_Entitled, etc.
}

interface AddressConfig {
  addrType: string;  // "1" = FQDN, "2" = IPv4, "3" = IPv6
  addr: string;
}
```

### Step 2: JSON response builder — `src/protocol/jsonBuilder.ts` (new)

Converts `ServiceEntitlementResponse` → TS.43 JSON format:

```json
{
  "Vers": { "version": "1", "validity": "172800" },
  "Token": { "token": "..." },
  "ap2004": {
    "EntitlementStatus": "1",
    "AddrStatus": "1",
    "TC_Status": "0",
    "ProvStatus": "0",
    "Addr": { "1": { "AddrType": "1", "Addr": "epdg.operator.com" } }
  }
}
```

Export: `buildJsonResponse(response: ServiceEntitlementResponse): object`

### Step 3: XML response builder — `src/protocol/xmlBuilder.ts` (new)

Converts `ServiceEntitlementResponse` → WAP-Provisioning XML string:

```xml
<?xml version="1.0"?>
<wap-provisioningdoc version="1.1">
  <characteristic type="VERS">...</characteristic>
  <characteristic type="TOKEN">...</characteristic>
  <characteristic type="APPLICATION">...</characteristic>
</wap-provisioningdoc>
```

No external XML library — build the simple XML string manually (the format is rigid and predictable). Escape `&`, `<`, `>`, `"` in values.

Export: `buildXmlResponse(response: ServiceEntitlementResponse): string`

### Step 4: Service handlers — `src/services/` (new directory, 3 files)

Each handler takes the subscriber's entitlement record (from DB) and returns an `ApplicationConfig`.

**`src/services/vowifi.ts`** (ap2004):
- `buildVoWiFiConfig(entitlement, configData)` → `ApplicationConfig`
- Default addresses: `epdg.operator.com` (ePDG), `pcscf.operator.com` (P-CSCF)
- If status=ENABLED and provisioned → include addresses
- If TC_Status=REQUIRES_ACCEPTANCE → include `ServiceFlow_URL`

**`src/services/volte.ts`** (ap2003):
- `buildVoLTEConfig(entitlement, configData)` → `ApplicationConfig`
- Extra params: `VoLTE_Entitled`, `VoNR_Entitled`
- Same address pattern as VoWiFi

**`src/services/smsoip.ts`** (ap2005):
- `buildSmsOipConfig(entitlement, configData)` → `ApplicationConfig`
- Minimal: just status + addresses

### Step 5: Response builder orchestrator — `src/protocol/responseBuilder.ts` (new)

Ties the pieces together:
- `buildEntitlementResponse(token, subscriberId, appId, contentType, configValidity)` → formatted response
- Looks up entitlement from DB
- Routes to the appropriate service handler based on appId
- Calls JSON or XML builder based on `accept_content_type` (default: JSON)
- Returns `{ body, contentType }` — the body is either an object (JSON) or string (XML)

### Step 6: Modify entitlement route — `src/server/routes/entitlement.ts` (modify)

Replace the inline `fetchEntitlements()` with calls to `buildEntitlementResponse()`.
- POST fast-auth path: use response builder
- POST EAP-AKA success path: use response builder
- GET path: use response builder
- Set `Content-Type: application/xml` for XML responses
- Pass `accept_content_type` from request body/query

### Step 7: Entitlement seed script — `src/db/seed-entitlements.ts` (new)

Seed entitlement records for the two test subscribers:

**Subscriber 1 (001010000000001 "Alice"):**
- ap2004 (VoWiFi): ENABLED, provisioned, addresses configured
- ap2003 (VoLTE): ENABLED, VoLTE+VoNR entitled
- ap2005 (SMSoIP): ENABLED

**Subscriber 2 (001010000000002 "Bob"):**
- ap2004 (VoWiFi): DISABLED, TC_Status=REQUIRES_ACCEPTANCE
- ap2003 (VoLTE): ENABLED
- ap2005 (SMSoIP): not configured (uses defaults)

Config data stored in `config_data` JSONB with addresses, service flow URLs, etc.

Add `"db:seed-entitlements"` script to `package.json`.

### Step 8: Tests

**`tests/unit/responseBuilder.test.ts`** (new):
- JSON builder produces correct TS.43 envelope (Vers, Token, app block)
- XML builder produces valid WAP-Provisioning XML
- Address arrays serialize correctly in both formats
- Empty addresses omitted when status=DISABLED
- Content type routing (json vs xml)

**`tests/integration/eapAka.integration.test.ts`** (modify):
- Add test: successful auth returns TS.43 formatted response with Vers/Token/app blocks
- Add test: XML response when `accept_content_type: 'xml'`

## Files (11 total)

```
src/protocol/
  responseTypes.ts       (new) — shared interfaces
  jsonBuilder.ts         (new) — JSON serializer
  xmlBuilder.ts          (new) — XML serializer
  responseBuilder.ts     (new) — orchestrator

src/services/
  vowifi.ts              (new) — VoWiFi handler
  volte.ts               (new) — VoLTE/VoNR handler
  smsoip.ts              (new) — SMSoIP handler

src/db/
  seed-entitlements.ts   (new) — entitlement seed data

src/server/routes/
  entitlement.ts         (modify) — use response builder

tests/unit/
  responseBuilder.test.ts (new) — format tests

tests/integration/
  eapAka.integration.test.ts (modify) — add format assertions
```

## Verification

1. `npm run build` — compiles
2. `npm run test:unit` — response builder tests pass (JSON + XML round-trips)
3. `docker compose up -d` + seed entitlements
4. `npm run test:integration` — formatted responses returned
5. Manual curl:
   ```bash
   # JSON response (default)
   curl -s -X POST http://localhost:8443/entitlement \
     -H 'Content-Type: application/json' \
     -d '{"app":"ap2004","terminal_id":"12345678901234","entitlement_version":"2","imsi":"001010000000001"}'
   # → 401 with TS.43 JSON envelope

   # XML response
   curl -s -X POST http://localhost:8443/entitlement \
     -H 'Content-Type: application/json' \
     -d '{"app":"ap2004","terminal_id":"12345678901234","entitlement_version":"2","token":"...","accept_content_type":"xml"}'
   # → 200 with WAP-Provisioning XML
   ```
