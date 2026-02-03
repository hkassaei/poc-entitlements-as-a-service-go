/** AppID values from GSMA TS.43 */
export const APP_IDS = {
  VOWIFI: 'ap2004',
  VOLTE: 'ap2003',
  SMSOIP: 'ap2005',
  ODSA_COMPANION: 'ap2006',
  ODSA_PRIMARY: 'ap2009',
  DATA_PLAN_INFO: 'ap2010',
  SERVER_INITIATED_ODSA: 'ap2011',
  DIRECT_CARRIER_BILLING: 'ap2012',
  PRIVATE_USER_IDENTITY: 'ap2013',
  DEVICE_USER_INFO: 'ap2014',
  APP_AUTHENTICATION: 'ap2015',
  SATELLITE_MODE: 'ap2016',
} as const;

export const ALL_APP_IDS = Object.values(APP_IDS);

/** HTTP status codes used in the TS.43 spec */
export const HTTP_STATUS = {
  OK: 200,
  FOUND: 302,
  BAD_REQUEST: 400,
  UNAUTHORIZED: 401,
  FORBIDDEN: 403,
  METHOD_NOT_ALLOWED: 405,
  NOT_ACCEPTABLE: 406,
  INTERNAL_SERVER_ERROR: 500,
  NOT_IMPLEMENTED: 501,
  SERVICE_UNAVAILABLE: 503,
} as const;

/** EAP-AKA protocol constants (RFC 4187) */
export const EAP_AKA = {
  SESSION_TTL_SECONDS: 90,
  IDEMPOTENCY_TTL_SECONDS: 90,
  AT_RAND: 1,
  AT_AUTN: 2,
  AT_RES: 3,
  AT_AUTS: 4,
  AT_MAC: 11,
} as const;

/** EAP-AKA session states */
export const EAP_STATE = {
  IDLE: 'IDLE',
  CHALLENGE_SENT: 'CHALLENGE_SENT',
  SUCCESS: 'SUCCESS',
  FAILURE: 'FAILURE',
  SYNC_FAILURE: 'SYNC_FAILURE',
} as const;

/** Token types */
export const TOKEN_TYPES = {
  AUTH: 'auth',
  FAST_AUTH: 'fast_auth',
  TEMPORARY: 'temporary',
  OPERATOR: 'operator',
} as const;
