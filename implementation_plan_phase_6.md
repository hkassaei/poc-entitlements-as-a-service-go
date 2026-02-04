# Phase 6: ODSA Flows

## Overview

Add eSIM lifecycle management via ODSA (On-Device Service Activation) for companion devices (ap2006) and primary devices (ap2009). Unlike VoWiFi/VoLTE/SMSoIP which are simple entitlement lookups, ODSA operations are **stateful workflows**: a device checks eligibility, subscribes, then downloads an eSIM profile.

The key design decision is that ODSA fits within the existing pipeline. The `operation` field (already in the request schema) acts as a sub-router within the service handler. The entitlements table's `configData` JSONB stores ODSA workflow state. The existing `extraParams` on `ApplicationConfig` carries ODSA-specific response fields through the JSON/XML builders unchanged.

## Implementation Steps

### Step 1: Fix SubscriptionResult to use numeric codes — `src/protocol/statusCodes.ts` (modify)

The TS.43 spec uses numeric codes for SubscriptionResult. Update from string labels to numeric values:

```typescript
export const SubscriptionResult = {
  CONTINUE_TO_WS: 1,
  DOWNLOAD_PROFILE: 2,
  DONE: 3,
  DELAYED_DOWNLOAD: 4,
  DELETE_PROFILE_IN_USE: 6,
  REQUIRES_USER_INPUT: 7,
} as const;
```

### Step 2: Mock SM-DP+ — `src/services/mockSmdp.ts` (new)

Pure in-process module (not a separate HTTP service) that returns canned eSIM activation codes and profile metadata:

```typescript
export interface ActivationCodeResponse {
  activationCode: string;   // "1$smdp.operator.com$matchingId"
  iccid: string;
  smdpAddress: string;
  profileType: string;
  matchingId: string;
}

export function getActivationCode(profileKey: string): ActivationCodeResponse
export function listAvailableProfiles(): ActivationCodeResponse[]
```

Three canned profiles: `default` (postpaid), `prepaid`, `companion`.

### Step 3: ODSA shared types and helpers — `src/services/odsaCommon.ts` (new)

Shared interface for what goes in `configData` JSONB for ap2006/ap2009:

```typescript
export interface OdsaConfigData {
  subscriptionState?: 'eligible' | 'pending' | 'active' | 'suspended';
  smdpAddress?: string;
  profileIccid?: string;        // key into mockSmdp canned profiles
  profileType?: string;
  companionDeviceImei?: string;  // ap2006 only
  primaryDeviceImsi?: string;    // ap2006 only
  planId?: string;               // ap2009 only
  planName?: string;             // ap2009 only
  serviceFlowUrl?: string;
  serviceFlowUserData?: string;
}
```

Helper to build a base ODSA response with SubscriptionResult in extraParams:

```typescript
export function buildOdsaBaseConfig(
  appId: string, status: number, provStatus: number, tcStatus: number,
  subscriptionResult: number, extraFields?: Record<string, string>,
): ApplicationConfig
```

### Step 4: Companion device handler — `src/services/odsaCompanion.ts` (new)

```typescript
export function buildCompanionConfig(
  status: number, provStatus: number, tcStatus: number,
  configData?: unknown, odsaContext?: OdsaContext,
): ApplicationConfig
```

Operations routed by `odsaContext.operation`:

| Operation | Behavior |
|-----------|----------|
| `CheckEligibility` | If subscriptionState is eligible/active → ENABLED + DONE; otherwise DISABLED + DONE |
| `ManageSubscription` | If eligible + serviceFlowUrl → CONTINUE_TO_WS; if active + smdpAddress → DOWNLOAD_PROFILE with SM-DP+ activation code; else DONE |
| `ManageService` | Return current status + DONE |
| `AcquireConfiguration` | Return SM-DP+ address and profile ICCID if available |
| `AcquireTemporaryToken` | Return DONE (actual token creation handled in route layer) |
| No operation | Basic entitlement status only |

### Step 5: Primary device handler — `src/services/odsaPrimary.ts` (new)

Same as companion plus `AcquirePlan`:

```typescript
export function buildPrimaryConfig(
  status: number, provStatus: number, tcStatus: number,
  configData?: unknown, odsaContext?: OdsaContext,
): ApplicationConfig
```

`AcquirePlan`: if serviceFlowUrl → CONTINUE_TO_WS with plan info; otherwise DONE with planId/planName in extraParams.

### Step 6: Add OdsaContext and routing — `src/protocol/responseBuilder.ts` (modify)

Add optional `OdsaContext` parameter:

```typescript
export interface OdsaContext {
  operation?: string;
  operationType?: number;
}
```

- `buildEntitlementResponse()` gains optional `odsaContext` parameter (backward-compatible)
- `buildAppConfig()` gains optional `odsaContext` parameter, adds two switch cases:
  - `'ap2006'` → `buildCompanionConfig(status, provStatus, tcStatus, configData, odsaContext)`
  - `'ap2009'` → `buildPrimaryConfig(status, provStatus, tcStatus, configData, odsaContext)`

### Step 7: Add `AcquirePlan` to request schema — `src/protocol/requestSchemas.ts` (modify)

Add `Type.Literal('AcquirePlan')` to `OdsaOperationSchema` union (both body and query).

### Step 8: Temporary token generation — `src/auth/tokenService.ts` (modify)

Add one new function (existing functions unchanged):

```typescript
export async function generateTemporaryToken(
  subscriberId: string,
  clientIp: string,
  scope: string,
  operationTargets: string[],
): Promise<TokenInfo>
```

- Uses `TOKEN_TYPES.TEMPORARY` and `config.tempTokenTtlSeconds`
- Stores `scope` and `operationTargets` in both Postgres and Redis cache
- Returns standard `TokenInfo`

### Step 9: Route handler changes — `src/server/routes/entitlement.ts` (modify)

Two changes:

**1. Pass ODSA context to response builder** — in all three auth paths:

```typescript
const formatted = await buildEntitlementResponse(
  token, subscriberId, body.app, body.accept_content_type,
  { operation: body.operation, operationType: body.operation_type },
);
```

**2. Handle `AcquireTemporaryToken` side effect** — after building the response for the token-auth path, if `operation === 'AcquireTemporaryToken'` and app is ap2006/ap2009:

- Call `generateTemporaryToken(subscriberId, clientIp, scope, operationTargets)`
- Inject `TemporaryToken` and `TemporaryTokenValidity` into the response's extraParams (for JSON) or as flat fields

### Step 10: Seed ODSA entitlements — `src/db/seed-entitlements.ts` (modify)

Add four ODSA entitlement records:

| Subscriber | App | State | Scenario |
|-----------|-----|-------|----------|
| Alice (001010000000001) | ap2006 | active + smdpAddress | ManageSubscription → DOWNLOAD_PROFILE |
| Alice (001010000000001) | ap2009 | eligible + serviceFlowUrl | CheckEligibility → eligible, AcquirePlan → CONTINUE_TO_WS |
| Bob (001010000000002) | ap2006 | eligible + serviceFlowUrl | ManageSubscription → CONTINUE_TO_WS |
| Bob (001010000000002) | ap2009 | active + smdpAddress | ManageSubscription → DOWNLOAD_PROFILE |

### Step 11: Unit tests — `tests/unit/odsa.test.ts` (new)

**Mock SM-DP+ tests:**
- `getActivationCode('default')` returns expected canned response
- `getActivationCode('unknown')` falls back to default
- `listAvailableProfiles()` returns all profiles

**Companion (ap2006) handler tests:**
- CheckEligibility with subscriptionState='eligible' → ENABLED + DONE
- CheckEligibility with no subscriptionState → DISABLED + DONE
- ManageSubscription with eligible + serviceFlowUrl → CONTINUE_TO_WS + ServiceFlow_URL
- ManageSubscription with active + smdpAddress → DOWNLOAD_PROFILE + SMDP+Address + activation code
- ManageSubscription with active, no smdpAddress → DONE
- ManageService → DONE with current status
- AcquireConfiguration with smdpAddress → includes SMDP+Address
- No operation → basic status response

**Primary (ap2009) handler tests:**
- Same as companion plus:
- AcquirePlan with serviceFlowUrl → CONTINUE_TO_WS
- AcquirePlan with planId/planName → includes plan fields in extraParams

### Step 12: Integration tests — `tests/integration/odsa.integration.test.ts` (new)

**Companion flow (ap2006):**
- Authenticate via token, CheckEligibility → 200 with SubscriptionResult
- ManageSubscription for active subscriber → DOWNLOAD_PROFILE with SM-DP+ activation code
- ManageSubscription for eligible subscriber → CONTINUE_TO_WS with ServiceFlow_URL
- AcquireTemporaryToken → 200 with TemporaryToken in response

**Primary flow (ap2009):**
- AcquirePlan → CONTINUE_TO_WS with plan info
- ManageSubscription for active subscriber → DOWNLOAD_PROFILE

**Content type:**
- XML response for ODSA includes SubscriptionResult in WAP-Provisioning format

**Error cases:**
- Unknown operation → basic status response
- No entitlement for subscriber+app → uses defaults

## Files (12 total)

```
src/services/
  mockSmdp.ts            (new)    — canned SM-DP+ activation codes
  odsaCommon.ts           (new)    — shared ODSA types + helpers
  odsaCompanion.ts        (new)    — ap2006 companion device handler
  odsaPrimary.ts          (new)    — ap2009 primary device handler

src/protocol/
  statusCodes.ts          (modify) — fix SubscriptionResult to numeric codes
  requestSchemas.ts       (modify) — add AcquirePlan operation
  responseBuilder.ts      (modify) — add OdsaContext + ap2006/ap2009 routing

src/auth/
  tokenService.ts         (modify) — add generateTemporaryToken()

src/server/routes/
  entitlement.ts          (modify) — pass OdsaContext, handle AcquireTemporaryToken

src/db/
  seed-entitlements.ts    (modify) — add ODSA seed records

tests/unit/
  odsa.test.ts            (new)    — SM-DP+ + companion + primary handler tests

tests/integration/
  odsa.integration.test.ts (new)   — full ODSA flow tests
```

## Design Notes

**AcquireTemporaryToken crosses layers.** Token generation is a side effect that can't happen in a pure service handler. The solution: the service handler returns the SubscriptionResult, and the route layer generates the temporary token and injects it into the response afterward.

**SubscriptionResult uses TS.43 numeric codes.** The current `statusCodes.ts` uses string labels — we change to numeric values (1=CONTINUE_TO_WS, 2=DOWNLOAD_PROFILE, 3=DONE, etc.) since that's what the spec requires and what goes over the wire.

**Backward compatible.** The `odsaContext` parameter is optional on `buildEntitlementResponse()` and `buildAppConfig()`. Existing VoWiFi/VoLTE/SMSoIP paths don't pass it and their handlers ignore it.

## Verification

1. `npx tsc --noEmit` — compiles
2. `npm run test:unit` — all unit tests pass including ODSA handlers
3. `npm run db:seed-entitlements` — seeds ODSA entitlement records
4. `npm run test:integration` — ODSA flows return correct SubscriptionResult codes and SM-DP+ data
5. Manual curl:
   ```bash
   # CheckEligibility for companion device
   curl -s -X POST http://localhost:8443/entitlement \
     -H 'Content-Type: application/json' \
     -d '{"app":"ap2006","terminal_id":"12345678901234","entitlement_version":"2","token":"...","operation":"CheckEligibility"}'

   # ManageSubscription → DOWNLOAD_PROFILE
   curl -s -X POST http://localhost:8443/entitlement \
     -H 'Content-Type: application/json' \
     -d '{"app":"ap2006","terminal_id":"12345678901234","entitlement_version":"2","token":"...","operation":"ManageSubscription"}'
   ```
