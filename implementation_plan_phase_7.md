# Phase 7: Extended Services Implementation Plan

## Overview
Add 7 new service handlers (ap2010–ap2016) following the established builder pattern, register them in the response router, add seed data, and write unit tests.

---

## Files to Create

### 1. `src/services/dataPlan.ts` (ap2010 — Data Plan Information)
- `buildDataPlanConfig(status, provStatus, tcStatus, configData, odsaContext)`
- Operation-aware (like ODSA): supports `CheckEligibility`, `AcquirePlan`, `GetPlanDetails`
- ConfigData interface: `planName`, `planId`, `dataAllowanceBytes`, `dataUsedBytes`, `billingCycleEnd`, `accessType`, `dataType` (metered/unmetered), `boostEligible`
- When ENABLED: populates extraParams with plan details
- Uses `odsaContext` for operation routing since data plan has operations

### 2. `src/services/serverOdsa.ts` (ap2011 — Server-Initiated ODSA)
- `buildServerOdsaConfig(status, provStatus, tcStatus, configData, odsaContext)`
- Operation-aware: supports `CheckEligibility`, `ManageSubscription`, `ManageService`
- ConfigData: `enterpriseId`, `smdpAddress`, `profileType`, `subscriptionState`, `serviceFlowUrl`
- Reuses `buildOdsaBaseConfig` from `odsaCommon.ts` (same SubscriptionResult pattern)
- Similar to companion/primary but scoped to enterprise context

### 3. `src/services/directCarrierBilling.ts` (ap2012 — DCB)
- `buildDcbConfig(status, provStatus, tcStatus, configData)`
- Simple builder (no operations): returns entitlement status + T&C status
- ConfigData: `serviceFlowUrl` (for T&C web portal)
- Decision matrix: INCOMPATIBLE → message, DISABLED + REQUIRES_ACCEPTANCE → websheet, ENABLED → can purchase

### 4. `src/services/privateUserIdentity.ts` (ap2013 — stub)
- `buildPrivateIdentityConfig(status, provStatus, tcStatus, configData)`
- ConfigData: `pseudonym`, `identityType` (PSEUDONYM | OTHER)
- When ENABLED: returns pseudonym or encrypted identity info in extraParams

### 5. `src/services/deviceUserInfo.ts` (ap2014 — stub)
- `buildDeviceUserInfoConfig(status, provStatus, tcStatus, configData, odsaContext)`
- Operation-aware: `GetPhoneNumber`, `GetSubscriberInfo`
- ConfigData: `msisdn`, `displayName`, `homeCarrier`
- Returns subscriber info fields in extraParams

### 6. `src/services/appAuthentication.ts` (ap2015 — stub)
- `buildAppAuthConfig(status, provStatus, tcStatus, configData)`
- ConfigData: `operatorTokenUrl`, `appTokenScope`
- When ENABLED: returns token endpoint info in extraParams

### 7. `src/services/satMode.ts` (ap2016 — stub)
- `buildSatModeConfig(status, provStatus, tcStatus, configData)`
- ConfigData: `plmnAllow` (string[]), `plmnBarred` (string[]), `serviceConstraints`
- When ENABLED: returns PLMN lists in extraParams

---

## Files to Modify

### 8. `src/protocol/responseBuilder.ts`
- Add imports for all 7 new service builders
- Add 7 case statements in `buildAppConfig()` switch for ap2010–ap2016

### 9. `src/db/seed-entitlements.ts`
- Add seed data for Alice and Bob covering ap2010–ap2016
- Alice: most services ENABLED with rich configData
- Bob: mix of ENABLED/DISABLED for testing different states

---

## Files to Create (Tests)

### 10. `tests/unit/extendedServices.test.ts`
- Unit tests for all 7 new builders
- Follow pattern from `tests/unit/odsa.test.ts`
- Test: enabled state with config, disabled state, operation routing (for ap2010/ap2011/ap2014), missing configData defaults

---

## Implementation Order

1. Create `src/services/dataPlan.ts` (ap2010) — richest of the new services
2. Create `src/services/serverOdsa.ts` (ap2011) — operation-aware, reuses odsaCommon
3. Create `src/services/directCarrierBilling.ts` (ap2012) — simple builder
4. Create stubs: `privateUserIdentity.ts`, `deviceUserInfo.ts`, `appAuthentication.ts`, `satMode.ts`
5. Update `responseBuilder.ts` — add all 7 cases
6. Update `seed-entitlements.ts` — add seed data
7. Create `tests/unit/extendedServices.test.ts`
8. Run tests to verify

## Verification
- `npm run build` — TypeScript compiles cleanly
- `npx vitest run tests/unit/extendedServices.test.ts` — all new unit tests pass
- `npx vitest run` — all existing tests still pass (no regressions)
