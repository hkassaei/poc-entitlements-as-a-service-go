import 'dotenv/config';

export interface Config {
  port: number;
  host: string;
  databaseUrl: string;
  dbPoolSize: number;
  localKekHex: string;
  gcpProjectId: string;
  kmsKeyRing: string;
  kmsKeyName: string;
  kmsLocation: string;
  nodeEnv: string;
}

function required(name: string): string {
  const value = process.env[name];
  if (!value) {
    throw new Error(`Missing required environment variable: ${name}`);
  }
  return value;
}

export const config: Config = {
  port: parseInt(process.env['PORT'] ?? '3001', 10),
  host: process.env['HOST'] ?? '0.0.0.0',
  databaseUrl: required('DATABASE_URL'),
  dbPoolSize: parseInt(process.env['DB_POOL_SIZE'] ?? '2', 10),
  localKekHex: process.env['LOCAL_KEK_HEX'] ?? '',
  gcpProjectId: process.env['GCP_PROJECT_ID'] ?? '',
  kmsKeyRing: process.env['KMS_KEY_RING'] ?? 'entitlement-keys',
  kmsKeyName: process.env['KMS_KEY_NAME'] ?? 'ki-kek',
  kmsLocation: process.env['KMS_LOCATION'] ?? 'us-central1',
  nodeEnv: process.env['NODE_ENV'] ?? 'development',
};
