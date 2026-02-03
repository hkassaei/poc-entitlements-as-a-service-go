import pino from 'pino';
import { config } from './index.js';

/**
 * Cloud Logging severity mapping:
 * pino trace (10) → DEBUG
 * pino debug (20) → DEBUG
 * pino info (30) → INFO
 * pino warn (40) → WARNING
 * pino error (50) → ERROR
 * pino fatal (60) → CRITICAL
 */
const severityMap: Record<number, string> = {
  10: 'DEBUG',
  20: 'DEBUG',
  30: 'INFO',
  40: 'WARNING',
  50: 'ERROR',
  60: 'CRITICAL',
};

export const logger = pino({
  level: config.nodeEnv === 'production' ? 'info' : 'debug',
  formatters: {
    level(label, number) {
      return {
        severity: severityMap[number] ?? 'DEFAULT',
        level: label,
      };
    },
  },
  mixin() {
    // Stub for OpenTelemetry trace context correlation.
    // Will be populated when tracing is fully wired.
    return {};
  },
  timestamp: pino.stdTimeFunctions.isoTime,
});
