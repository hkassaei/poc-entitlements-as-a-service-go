/**
 * Mock SM-DP+ Service
 *
 * In-process module that returns canned eSIM activation codes
 * and profile metadata. Not a separate HTTP service.
 */

export interface ActivationCodeResponse {
  activationCode: string;
  iccid: string;
  smdpAddress: string;
  profileType: string;
  matchingId: string;
}

const PROFILES: Record<string, ActivationCodeResponse> = {
  default: {
    activationCode: '1$smdp.operator.com$POSTPAID-001',
    iccid: '8901010000000000001',
    smdpAddress: 'smdp.operator.com',
    profileType: 'postpaid',
    matchingId: 'POSTPAID-001',
  },
  prepaid: {
    activationCode: '1$smdp.operator.com$PREPAID-001',
    iccid: '8901010000000000002',
    smdpAddress: 'smdp.operator.com',
    profileType: 'prepaid',
    matchingId: 'PREPAID-001',
  },
  companion: {
    activationCode: '1$smdp.operator.com$COMPANION-001',
    iccid: '8901010000000000003',
    smdpAddress: 'smdp.operator.com',
    profileType: 'companion',
    matchingId: 'COMPANION-001',
  },
};

/**
 * Get an activation code for a profile key.
 * Falls back to the 'default' profile if the key is unknown.
 */
export function getActivationCode(profileKey: string): ActivationCodeResponse {
  return PROFILES[profileKey] ?? PROFILES['default']!;
}

/**
 * List all available canned profiles.
 */
export function listAvailableProfiles(): ActivationCodeResponse[] {
  return Object.values(PROFILES);
}
