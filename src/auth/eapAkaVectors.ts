/**
 * HSS Vector Client
 *
 * Thin HTTP client that calls the mock-hss /vectors endpoint to obtain
 * MILENAGE authentication vectors for a given IMSI.
 */

import { config } from '../config/index.js';
import { logger } from '../config/logger.js';

export interface AuthVectors {
  rand: Buffer;
  autn: Buffer;
  xres: Buffer;
  ck: Buffer;
  ik: Buffer;
}

/**
 * Fetch authentication vectors from the HSS for the given IMSI.
 *
 * @throws Error if the subscriber is not found (404) or the HSS is unreachable.
 */
export async function fetchVectors(imsi: string): Promise<AuthVectors> {
  const url = `${config.hssUrl}/vectors`;

  const response = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ imsi }),
    signal: AbortSignal.timeout(5000),
  });

  if (response.status === 404) {
    throw new HssSubscriberNotFoundError(imsi);
  }

  if (!response.ok) {
    const text = await response.text().catch(() => '');
    logger.error({ imsi, status: response.status, body: text }, 'HSS request failed');
    throw new Error(`HSS returned ${response.status}`);
  }

  const data = (await response.json()) as {
    rand: string;
    autn: string;
    xres: string;
    ck: string;
    ik: string;
  };

  return {
    rand: Buffer.from(data.rand, 'base64'),
    autn: Buffer.from(data.autn, 'base64'),
    xres: Buffer.from(data.xres, 'base64'),
    ck: Buffer.from(data.ck, 'base64'),
    ik: Buffer.from(data.ik, 'base64'),
  };
}

export class HssSubscriberNotFoundError extends Error {
  public readonly imsi: string;
  constructor(imsi: string) {
    super(`Subscriber not found: ${imsi}`);
    this.name = 'HssSubscriberNotFoundError';
    this.imsi = imsi;
  }
}
