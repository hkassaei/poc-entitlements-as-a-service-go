import 'dotenv/config';

export interface Config {
  port: number;
  host: string;
  databaseUrl: string;
  dbPoolSize: number;
  localKekHex: string;
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
  dbPoolSize: parseInt(process.env['DB_POOL_SIZE'] ?? '5', 10),
  localKekHex: required('LOCAL_KEK_HEX'),
  nodeEnv: process.env['NODE_ENV'] ?? 'development',
};
