import { MeterProvider } from '@opentelemetry/sdk-metrics';
import { OTLPMetricExporter } from '@opentelemetry/exporter-metrics-otlp-http';
import { resourceFromAttributes } from '@opentelemetry/resources';
import { ATTR_SERVICE_NAME, ATTR_SERVICE_VERSION } from '@opentelemetry/semantic-conventions';

const isDev = process.env.NODE_ENV !== 'production';

const resource = resourceFromAttributes({
  [ATTR_SERVICE_NAME]: 'entitlements-ecs',
  [ATTR_SERVICE_VERSION]: '0.1.0',
});

const meterProvider = new MeterProvider({
  resource,
  readers: isDev
    ? []
    : [
        // In production, use periodic exporting metric reader.
        // Configured in later phases when custom metrics are added.
      ],
});

export const meter = meterProvider.getMeter('entitlements-ecs');

process.on('SIGTERM', () => {
  meterProvider.shutdown().catch(console.error);
});

// Keep the exporter reference for later use
export { OTLPMetricExporter, meterProvider };
