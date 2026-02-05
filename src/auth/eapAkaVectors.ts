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

export interface ResyncResult {
  success: boolean;
  vectors?: AuthVectors;
  error?: string;
}

/**
 * Request SQN resynchronization from the HSS after a SYNC_FAILURE.
 *
 * The HSS validates the AUTS, extracts the device's SQN_MS,
 * updates its sequence counter, and returns fresh auth vectors.
 *
 * @param imsi The subscriber's IMSI
 * @param rand The RAND from the failed challenge (16 bytes)
 * @param auts The AT_AUTS from the SYNC_FAILURE (14 bytes)
 * @returns ResyncResult with fresh vectors on success, or error details on failure
 */
export async function resyncVectors(
  imsi: string,
  rand: Buffer,
  auts: Buffer,
): Promise<ResyncResult> {
  const url = `${config.hssUrl}/resync`;

  try {
    const response = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        imsi,
        rand: rand.toString('base64'),
        auts: auts.toString('base64'),
      }),
      signal: AbortSignal.timeout(5000),
    });

    if (response.status === 404) {
      return { success: false, error: 'Subscriber not found' };
    }

    if (response.status === 400) {
      const data = (await response.json()) as { error?: string };
      return { success: false, error: data.error ?? 'AUTS validation failed' };
    }

    if (!response.ok) {
      const text = await response.text().catch(() => '');
      logger.error({ imsi, status: response.status, body: text }, 'HSS resync request failed');
      return { success: false, error: `HSS returned ${response.status}` };
    }

    const data = (await response.json()) as {
      rand: string;
      autn: string;
      xres: string;
      ck: string;
      ik: string;
    };

    return {
      success: true,
      vectors: {
        rand: Buffer.from(data.rand, 'base64'),
        autn: Buffer.from(data.autn, 'base64'),
        xres: Buffer.from(data.xres, 'base64'),
        ck: Buffer.from(data.ck, 'base64'),
        ik: Buffer.from(data.ik, 'base64'),
      },
    };
  } catch (err) {
    logger.error({ imsi, err }, 'HSS resync request error');
    return { success: false, error: 'HSS communication error' };
  }
}
