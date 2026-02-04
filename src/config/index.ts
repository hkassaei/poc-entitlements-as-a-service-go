export interface Config {
  port: number;
  host: string;
  databaseUrl: string;
  dbPoolSize: number;
  redisUrl: string;
  hssUrl: string;
  authTokenTtlSeconds: number;
  fastAuthTokenTtlSeconds: number;
  tempTokenTtlSeconds: number;
  kmsKeyRing: string;
  kmsKeyName: string;
  kmsLocation: string;
  localKekHex: string;
  supportedVersions: string[];
  defaultConfigValidity: number;
  operatorMcc: string;
  operatorMnc: string;
  operatorName: string;
  gcpProjectId: string;
  nodeEnv: string;
}

function envOrDefault(key: string, fallback: string): string {
  return process.env[key] ?? fallback;
}

export function loadConfig(): Config {
  return {
    port: parseInt(envOrDefault('PORT', '8080'), 10),
    host: envOrDefault('HOST', '0.0.0.0'),
    databaseUrl: envOrDefault('DATABASE_URL', 'postgresql://ecs:password@localhost:5432/entitlements'),
    dbPoolSize: parseInt(envOrDefault('DB_POOL_SIZE', '2'), 10),
    redisUrl: envOrDefault('REDIS_URL', 'redis://localhost:6379'),
    hssUrl: envOrDefault('HSS_URL', 'http://mock-hss:3001'),
    authTokenTtlSeconds: parseInt(envOrDefault('AUTH_TOKEN_TTL_SECONDS', '86400'), 10),
    fastAuthTokenTtlSeconds: parseInt(envOrDefault('FAST_AUTH_TOKEN_TTL_SECONDS', '172800'), 10),
    tempTokenTtlSeconds: parseInt(envOrDefault('TEMP_TOKEN_TTL_SECONDS', '3600'), 10),
    kmsKeyRing: envOrDefault('KMS_KEY_RING', 'entitlement-keys'),
    kmsKeyName: envOrDefault('KMS_KEY_NAME', 'ki-kek'),
    kmsLocation: envOrDefault('KMS_LOCATION', 'us-central1'),
    localKekHex: envOrDefault('LOCAL_KEK_HEX', ''),
    supportedVersions: envOrDefault('SUPPORTED_VERSIONS', '2,4').split(',').map(v => v.trim()),
    defaultConfigValidity: parseInt(envOrDefault('DEFAULT_CONFIG_VALIDITY', '172800'), 10),
    operatorMcc: envOrDefault('OPERATOR_MCC', '001'),
    operatorMnc: envOrDefault('OPERATOR_MNC', '01'),
    operatorName: envOrDefault('OPERATOR_NAME', 'TestOperator'),
    gcpProjectId: envOrDefault('GCP_PROJECT_ID', ''),
    nodeEnv: envOrDefault('NODE_ENV', 'development'),
  };
}

export const config = loadConfig();
