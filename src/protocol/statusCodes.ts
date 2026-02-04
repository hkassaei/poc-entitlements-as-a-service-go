/** Entitlement status (per-service) */
export const EntitlementStatus = {
  DISABLED: 0,
  ENABLED: 1,
  INCOMPATIBLE: 2,
  PROVISIONING: 3,
} as const;

/** Service status */
export const ServiceStatus = {
  NOT_CONFIGURED: 0,
  ENABLED: 1,
  DISABLED: 2,
  NOT_SUBSCRIBED: 3,
} as const;

/** ODSA subscription result (TS.43 numeric codes) */
export const SubscriptionResult = {
  CONTINUE_TO_WS: 1,
  DOWNLOAD_PROFILE: 2,
  DONE: 3,
  DELAYED_DOWNLOAD: 4,
  DELETE_PROFILE_IN_USE: 6,
  REQUIRES_USER_INPUT: 7,
} as const;

/** Terms & Conditions status */
export const TcStatus = {
  NOT_PROVIDED: 0,
  ACCEPTED: 1,
  REQUIRES_ACCEPTANCE: 2,
  REJECTED: 3,
} as const;

/** Provisioning status */
export const ProvStatus = {
  NOT_NEEDED: 0,
  REQUIRED: 1,
  IN_PROGRESS: 2,
  COMPLETE: 3,
} as const;
