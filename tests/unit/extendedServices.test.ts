import { describe, it, expect } from 'vitest';
import { buildDataPlanConfig } from '../../src/services/dataPlan.js';
import { buildServerOdsaConfig } from '../../src/services/serverOdsa.js';
import { buildDcbConfig } from '../../src/services/directCarrierBilling.js';
import { buildPrivateIdentityConfig } from '../../src/services/privateUserIdentity.js';
import { buildDeviceUserInfoConfig } from '../../src/services/deviceUserInfo.js';
import { buildAppAuthConfig } from '../../src/services/appAuthentication.js';
import { buildSatModeConfig } from '../../src/services/satMode.js';
import { SubscriptionResult } from '../../src/protocol/statusCodes.js';

// ---------------------------------------------------------------------------
// ap2010 — Data Plan
// ---------------------------------------------------------------------------
describe('Data Plan (ap2010)', () => {
  it('no operation → basic status', () => {
    const config = buildDataPlanConfig(1, 3, 1, {});
    expect(config.appId).toBe('ap2010');
    expect(config.entitlementStatus).toBe(1);
    expect(config.extraParams).toBeUndefined();
  });

  it('CheckEligibility with planId → ENABLED', () => {
    const config = buildDataPlanConfig(1, 0, 0, { planId: 'PLAN-001' }, { operation: 'CheckEligibility' });
    expect(config.entitlementStatus).toBe(1);
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.DONE));
  });

  it('CheckEligibility with no planId and not boostEligible → DISABLED', () => {
    const config = buildDataPlanConfig(1, 0, 0, {}, { operation: 'CheckEligibility' });
    expect(config.entitlementStatus).toBe(0);
  });

  it('AcquirePlan with serviceFlowUrl → CONTINUE_TO_WS', () => {
    const config = buildDataPlanConfig(1, 0, 0, {
      serviceFlowUrl: 'https://operator.com/plans',
      planId: 'PLAN-001',
      planName: 'Unlimited',
    }, { operation: 'AcquirePlan' });
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.CONTINUE_TO_WS));
    expect(config.extraParams?.ServiceFlow_URL).toBe('https://operator.com/plans');
    expect(config.extraParams?.PlanId).toBe('PLAN-001');
    expect(config.extraParams?.PlanName).toBe('Unlimited');
  });

  it('AcquirePlan without serviceFlowUrl → DONE with plan fields', () => {
    const config = buildDataPlanConfig(1, 0, 0, {
      planId: 'PLAN-002',
    }, { operation: 'AcquirePlan' });
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.DONE));
    expect(config.extraParams?.PlanId).toBe('PLAN-002');
  });

  it('GetPlanDetails returns all plan fields', () => {
    const config = buildDataPlanConfig(1, 0, 0, {
      planId: 'PLAN-001',
      planName: 'Unlimited Plus',
      dataAllowanceBytes: 107374182400,
      dataUsedBytes: 21474836480,
      billingCycleEnd: '2026-03-01',
      accessType: '5G',
      dataType: 'metered',
      boostEligible: true,
    }, { operation: 'GetPlanDetails' });
    expect(config.extraParams?.PlanId).toBe('PLAN-001');
    expect(config.extraParams?.PlanName).toBe('Unlimited Plus');
    expect(config.extraParams?.DataAllowanceBytes).toBe('107374182400');
    expect(config.extraParams?.DataUsedBytes).toBe('21474836480');
    expect(config.extraParams?.BillingCycleEnd).toBe('2026-03-01');
    expect(config.extraParams?.AccessType).toBe('5G');
    expect(config.extraParams?.DataType).toBe('metered');
    expect(config.extraParams?.BoostEligible).toBe('true');
  });

  it('GetPlanDetails with missing configData → empty extras besides SubscriptionResult', () => {
    const config = buildDataPlanConfig(1, 0, 0, undefined, { operation: 'GetPlanDetails' });
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.DONE));
    expect(config.extraParams?.PlanId).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// ap2011 — Server ODSA
// ---------------------------------------------------------------------------
describe('Server ODSA (ap2011)', () => {
  it('no operation → basic status', () => {
    const config = buildServerOdsaConfig(1, 0, 0, {});
    expect(config.appId).toBe('ap2011');
    expect(config.extraParams).toBeUndefined();
  });

  it('CheckEligibility with eligible → ENABLED + enterpriseId', () => {
    const config = buildServerOdsaConfig(1, 0, 0, {
      subscriptionState: 'eligible',
      enterpriseId: 'ENT-001',
    }, { operation: 'CheckEligibility' });
    expect(config.entitlementStatus).toBe(1);
    expect(config.extraParams?.EnterpriseId).toBe('ENT-001');
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.DONE));
  });

  it('CheckEligibility with pending → DISABLED', () => {
    const config = buildServerOdsaConfig(1, 0, 0, {
      subscriptionState: 'pending',
    }, { operation: 'CheckEligibility' });
    expect(config.entitlementStatus).toBe(0);
  });

  it('ManageSubscription with eligible + serviceFlowUrl → CONTINUE_TO_WS', () => {
    const config = buildServerOdsaConfig(1, 0, 0, {
      subscriptionState: 'eligible',
      serviceFlowUrl: 'https://operator.com/enterprise/setup',
      enterpriseId: 'ENT-001',
    }, { operation: 'ManageSubscription' });
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.CONTINUE_TO_WS));
    expect(config.extraParams?.ServiceFlow_URL).toBe('https://operator.com/enterprise/setup');
    expect(config.extraParams?.EnterpriseId).toBe('ENT-001');
  });

  it('ManageSubscription with active + smdpAddress → DOWNLOAD_PROFILE', () => {
    const config = buildServerOdsaConfig(1, 0, 0, {
      subscriptionState: 'active',
      smdpAddress: 'smdp.operator.com',
    }, { operation: 'ManageSubscription' });
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.DOWNLOAD_PROFILE));
    expect(config.extraParams?.['SMDP+Address']).toBe('smdp.operator.com');
    expect(config.extraParams?.['SMDP+ActivationCode']).toBeDefined();
    expect(config.extraParams?.ProfileICCID).toBeDefined();
  });

  it('ManageService → DONE', () => {
    const config = buildServerOdsaConfig(1, 3, 1, {}, { operation: 'ManageService' });
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.DONE));
  });
});

// ---------------------------------------------------------------------------
// ap2012 — Direct Carrier Billing
// ---------------------------------------------------------------------------
describe('Direct Carrier Billing (ap2012)', () => {
  it('ENABLED → basic status, no extra params', () => {
    const config = buildDcbConfig(1, 0, 1, {});
    expect(config.appId).toBe('ap2012');
    expect(config.entitlementStatus).toBe(1);
    expect(config.extraParams).toBeUndefined();
  });

  it('INCOMPATIBLE → message in extraParams', () => {
    const config = buildDcbConfig(2, 0, 0, {});
    expect(config.entitlementStatus).toBe(2);
    expect(config.extraParams?.Message).toContain('not available');
  });

  it('DISABLED + REQUIRES_ACCEPTANCE + serviceFlowUrl → serviceFlowUrl set', () => {
    const config = buildDcbConfig(0, 0, 2, { serviceFlowUrl: 'https://operator.com/terms/dcb' });
    expect(config.entitlementStatus).toBe(0);
    expect(config.serviceFlowUrl).toBe('https://operator.com/terms/dcb');
  });

  it('DISABLED without REQUIRES_ACCEPTANCE → no serviceFlowUrl', () => {
    const config = buildDcbConfig(0, 0, 0, { serviceFlowUrl: 'https://operator.com/terms/dcb' });
    expect(config.serviceFlowUrl).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// ap2013 — Private User Identity
// ---------------------------------------------------------------------------
describe('Private User Identity (ap2013)', () => {
  it('ENABLED with pseudonym → extraParams populated', () => {
    const config = buildPrivateIdentityConfig(1, 0, 0, {
      pseudonym: 'anon-abc123',
      identityType: 'PSEUDONYM',
    });
    expect(config.appId).toBe('ap2013');
    expect(config.extraParams?.Pseudonym).toBe('anon-abc123');
    expect(config.extraParams?.IdentityType).toBe('PSEUDONYM');
  });

  it('ENABLED with no pseudonym → default identityType', () => {
    const config = buildPrivateIdentityConfig(1, 0, 0, {});
    expect(config.extraParams?.IdentityType).toBe('PSEUDONYM');
  });

  it('DISABLED → no extraParams', () => {
    const config = buildPrivateIdentityConfig(0, 0, 0, { pseudonym: 'should-not-appear' });
    expect(config.extraParams).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// ap2014 — Device/User Info
// ---------------------------------------------------------------------------
describe('Device/User Info (ap2014)', () => {
  it('no operation → basic status', () => {
    const config = buildDeviceUserInfoConfig(1, 0, 0, {});
    expect(config.appId).toBe('ap2014');
    expect(config.extraParams).toBeUndefined();
  });

  it('GetPhoneNumber → MSISDN in extraParams', () => {
    const config = buildDeviceUserInfoConfig(1, 0, 0, {
      msisdn: '+15551234567',
    }, { operation: 'GetPhoneNumber' });
    expect(config.extraParams?.MSISDN).toBe('+15551234567');
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.DONE));
  });

  it('GetSubscriberInfo → all subscriber fields', () => {
    const config = buildDeviceUserInfoConfig(1, 0, 0, {
      msisdn: '+15551234567',
      displayName: 'Alice',
      homeCarrier: 'Test Operator',
    }, { operation: 'GetSubscriberInfo' });
    expect(config.extraParams?.MSISDN).toBe('+15551234567');
    expect(config.extraParams?.DisplayName).toBe('Alice');
    expect(config.extraParams?.HomeCarrier).toBe('Test Operator');
  });

  it('GetPhoneNumber with no msisdn → no MSISDN in extraParams', () => {
    const config = buildDeviceUserInfoConfig(1, 0, 0, {}, { operation: 'GetPhoneNumber' });
    expect(config.extraParams?.MSISDN).toBeUndefined();
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.DONE));
  });
});

// ---------------------------------------------------------------------------
// ap2015 — App Authentication
// ---------------------------------------------------------------------------
describe('App Authentication (ap2015)', () => {
  it('ENABLED with token info → extraParams populated', () => {
    const config = buildAppAuthConfig(1, 0, 0, {
      operatorTokenUrl: 'https://auth.operator.com/token',
      appTokenScope: 'carrier.entitlement',
    });
    expect(config.appId).toBe('ap2015');
    expect(config.extraParams?.OperatorTokenUrl).toBe('https://auth.operator.com/token');
    expect(config.extraParams?.AppTokenScope).toBe('carrier.entitlement');
  });

  it('ENABLED with no config → no extraParams', () => {
    const config = buildAppAuthConfig(1, 0, 0, {});
    expect(config.extraParams).toBeUndefined();
  });

  it('DISABLED → no extraParams even with config', () => {
    const config = buildAppAuthConfig(0, 0, 0, {
      operatorTokenUrl: 'https://auth.operator.com/token',
    });
    expect(config.extraParams).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// ap2016 — Satellite Mode
// ---------------------------------------------------------------------------
describe('Satellite Mode (ap2016)', () => {
  it('ENABLED with PLMN lists → extraParams populated', () => {
    const config = buildSatModeConfig(1, 0, 0, {
      plmnAllow: ['00101', '00102'],
      plmnBarred: ['99999'],
      serviceConstraints: 'sos-only',
    });
    expect(config.appId).toBe('ap2016');
    expect(config.extraParams?.PLMNAllow).toBe('00101,00102');
    expect(config.extraParams?.PLMNBarred).toBe('99999');
    expect(config.extraParams?.ServiceConstraints).toBe('sos-only');
  });

  it('ENABLED with empty arrays → no extraParams', () => {
    const config = buildSatModeConfig(1, 0, 0, { plmnAllow: [], plmnBarred: [] });
    expect(config.extraParams).toBeUndefined();
  });

  it('DISABLED → no extraParams even with config', () => {
    const config = buildSatModeConfig(0, 0, 0, {
      plmnAllow: ['00101'],
    });
    expect(config.extraParams).toBeUndefined();
  });

  it('ENABLED with no config → no extraParams', () => {
    const config = buildSatModeConfig(1, 0, 0, undefined);
    expect(config.extraParams).toBeUndefined();
  });
});
