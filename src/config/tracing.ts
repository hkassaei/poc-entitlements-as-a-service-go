import { NodeSDK } from '@opentelemetry/sdk-node';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http';
import { HttpInstrumentation } from '@opentelemetry/instrumentation-http';
import { PgInstrumentation } from '@opentelemetry/instrumentation-pg';
import { IORedisInstrumentation } from '@opentelemetry/instrumentation-ioredis';
import { resourceFromAttributes } from '@opentelemetry/resources';
import { ATTR_SERVICE_NAME, ATTR_SERVICE_VERSION } from '@opentelemetry/semantic-conventions';

const isDev = process.env.NODE_ENV !== 'production';

const resource = resourceFromAttributes({
  [ATTR_SERVICE_NAME]: 'entitlements-ecs',
  [ATTR_SERVICE_VERSION]: '0.1.0',
});

const sdk = new NodeSDK({
  resource,
  traceExporter: isDev ? undefined : new OTLPTraceExporter(),
  instrumentations: [
    new HttpInstrumentation(),
    new PgInstrumentation(),
    new IORedisInstrumentation(),
  ],
});

sdk.start();

process.on('SIGTERM', () => {
  sdk.shutdown().catch(console.error);
});

export { sdk };
