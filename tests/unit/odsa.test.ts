import { describe, it, expect } from 'vitest';
import { getActivationCode, listAvailableProfiles } from '../../src/services/mockSmdp.js';
import { buildCompanionConfig } from '../../src/services/odsaCompanion.js';
import { buildPrimaryConfig } from '../../src/services/odsaPrimary.js';
import { SubscriptionResult } from '../../src/protocol/statusCodes.js';

describe('Mock SM-DP+', () => {
  it('getActivationCode("default") returns postpaid profile', () => {
    const result = getActivationCode('default');
    expect(result.profileType).toBe('postpaid');
    expect(result.smdpAddress).toBe('smdp.operator.com');
    expect(result.activationCode).toContain('smdp.operator.com');
    expect(result.iccid).toBeDefined();
    expect(result.matchingId).toBeDefined();
  });

  it('getActivationCode("unknown") falls back to default', () => {
    const result = getActivationCode('unknown');
    expect(result).toEqual(getActivationCode('default'));
  });

  it('listAvailableProfiles() returns all profiles', () => {
    const profiles = listAvailableProfiles();
    expect(profiles.length).toBe(3);
    const types = profiles.map((p) => p.profileType);
    expect(types).toContain('postpaid');
    expect(types).toContain('prepaid');
    expect(types).toContain('companion');
  });
});

describe('ODSA Companion (ap2006)', () => {
  it('CheckEligibility with subscriptionState=eligible → ENABLED + DONE', () => {
    const config = buildCompanionConfig(1, 0, 0, { subscriptionState: 'eligible' }, { operation: 'CheckEligibility' });
    expect(config.appId).toBe('ap2006');
    expect(config.entitlementStatus).toBe(1); // ENABLED
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.DONE));
  });

  it('CheckEligibility with no subscriptionState → DISABLED + DONE', () => {
    const config = buildCompanionConfig(1, 0, 0, {}, { operation: 'CheckEligibility' });
    expect(config.entitlementStatus).toBe(0); // DISABLED
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.DONE));
  });

  it('ManageSubscription with eligible + serviceFlowUrl → CONTINUE_TO_WS', () => {
    const config = buildCompanionConfig(1, 0, 0, {
      subscriptionState: 'eligible',
      serviceFlowUrl: 'https://operator.com/setup',
    }, { operation: 'ManageSubscription' });
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.CONTINUE_TO_WS));
    expect(config.extraParams?.ServiceFlow_URL).toBe('https://operator.com/setup');
  });

  it('ManageSubscription with active + smdpAddress → DOWNLOAD_PROFILE + activation code', () => {
    const config = buildCompanionConfig(1, 0, 0, {
      subscriptionState: 'active',
      smdpAddress: 'smdp.operator.com',
      profileType: 'companion',
    }, { operation: 'ManageSubscription' });
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.DOWNLOAD_PROFILE));
    expect(config.extraParams?.['SMDP+Address']).toBe('smdp.operator.com');
    expect(config.extraParams?.['SMDP+ActivationCode']).toContain('smdp.operator.com');
    expect(config.extraParams?.ProfileICCID).toBeDefined();
  });

  it('ManageSubscription with active, no smdpAddress → DONE', () => {
    const config = buildCompanionConfig(1, 0, 0, {
      subscriptionState: 'active',
    }, { operation: 'ManageSubscription' });
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.DONE));
  });

  it('ManageService → DONE with current status', () => {
    const config = buildCompanionConfig(1, 3, 1, {}, { operation: 'ManageService' });
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.DONE));
    expect(config.entitlementStatus).toBe(1);
    expect(config.provStatus).toBe(3);
  });

  it('AcquireConfiguration with smdpAddress → includes SMDP+Address', () => {
    const config = buildCompanionConfig(1, 0, 0, {
      smdpAddress: 'smdp.operator.com',
    }, { operation: 'AcquireConfiguration' });
    expect(config.extraParams?.['SMDP+Address']).toBe('smdp.operator.com');
  });

  it('No operation → basic status response', () => {
    const config = buildCompanionConfig(1, 0, 0, {});
    expect(config.appId).toBe('ap2006');
    expect(config.entitlementStatus).toBe(1);
    expect(config.extraParams).toBeUndefined();
  });
});

describe('ODSA Primary (ap2009)', () => {
  it('CheckEligibility with subscriptionState=eligible → ENABLED + DONE', () => {
    const config = buildPrimaryConfig(1, 0, 0, { subscriptionState: 'eligible' }, { operation: 'CheckEligibility' });
    expect(config.appId).toBe('ap2009');
    expect(config.entitlementStatus).toBe(1);
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.DONE));
  });

  it('CheckEligibility with no subscriptionState → DISABLED + DONE', () => {
    const config = buildPrimaryConfig(1, 0, 0, {}, { operation: 'CheckEligibility' });
    expect(config.entitlementStatus).toBe(0);
  });

  it('ManageSubscription with active + smdpAddress → DOWNLOAD_PROFILE', () => {
    const config = buildPrimaryConfig(1, 0, 0, {
      subscriptionState: 'active',
      smdpAddress: 'smdp.operator.com',
    }, { operation: 'ManageSubscription' });
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.DOWNLOAD_PROFILE));
    expect(config.extraParams?.['SMDP+Address']).toBe('smdp.operator.com');
  });

  it('AcquirePlan with serviceFlowUrl → CONTINUE_TO_WS', () => {
    const config = buildPrimaryConfig(1, 0, 0, {
      serviceFlowUrl: 'https://operator.com/plans',
      planId: 'PLAN-001',
      planName: 'Unlimited',
    }, { operation: 'AcquirePlan' });
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.CONTINUE_TO_WS));
    expect(config.extraParams?.ServiceFlow_URL).toBe('https://operator.com/plans');
    expect(config.extraParams?.PlanId).toBe('PLAN-001');
    expect(config.extraParams?.PlanName).toBe('Unlimited');
  });

  it('AcquirePlan with planId/planName but no serviceFlowUrl → DONE with plan fields', () => {
    const config = buildPrimaryConfig(1, 0, 0, {
      planId: 'PLAN-002',
      planName: 'Basic',
    }, { operation: 'AcquirePlan' });
    expect(config.extraParams?.SubscriptionResult).toBe(String(SubscriptionResult.DONE));
    expect(config.extraParams?.PlanId).toBe('PLAN-002');
    expect(config.extraParams?.PlanName).toBe('Basic');
  });

  it('No operation → basic status response', () => {
    const config = buildPrimaryConfig(1, 0, 0, {});
    expect(config.appId).toBe('ap2009');
    expect(config.extraParams).toBeUndefined();
  });
});
