/**
 * RFC 4187 Section 7 — EAP-AKA Key Derivation
 *
 * Master Key:  MK = SHA-1(Identity | IK | CK)
 * PRF output:  K_encr(16) | K_aut(16) | MSK(64) | EMSK(64)  = 160 bytes
 * PRF uses the FIPS 186-2 SHA-1 based pseudo-random function.
 *
 * MAC computation uses HMAC-SHA-1 truncated to 16 bytes.
 */

import crypto from 'node:crypto';

/**
 * Build the permanent identity string for EAP-AKA.
 * Type "0" = permanent identity, followed by the IMSI.
 */
export function buildIdentity(imsi: string): string {
  return '0' + imsi;
}

/**
 * Derive the Master Key (MK) per RFC 4187 Section 7.
 * MK = SHA-1(Identity | IK | CK)
 */
export function deriveMasterKey(identity: string, ik: Buffer, ck: Buffer): Buffer {
  const hash = crypto.createHash('sha1');
  hash.update(Buffer.from(identity, 'utf-8'));
  hash.update(ik);
  hash.update(ck);
  return hash.digest();
}

/**
 * FIPS 186-2 SHA-1-based PRF.
 *
 * Generates `outputLength` bytes of pseudo-random output from a 20-byte seed.
 *
 * Algorithm (per iteration):
 *   1. w_i = SHA-1(xkey)
 *   2. xkey = (xkey + w_i + 1) mod 2^160
 *   3. Collect w_i, repeat
 */
export function prfSha1(mk: Buffer, outputLength: number): Buffer {
  const output = Buffer.alloc(outputLength);
  let offset = 0;

  // xkey is a 20-byte (160-bit) value, initialized from MK
  const xkey = Buffer.alloc(20);
  mk.copy(xkey, 0, 0, 20);

  while (offset < outputLength) {
    // w = SHA-1(xkey)
    const w = crypto.createHash('sha1').update(xkey).digest();

    // Copy w into output
    const toCopy = Math.min(20, outputLength - offset);
    w.copy(output, offset, 0, toCopy);
    offset += toCopy;

    // xkey = (xkey + w + 1) mod 2^160
    // Big-endian 160-bit addition
    add160(xkey, w);
    increment160(xkey);
  }

  return output;
}

/**
 * Add two 20-byte big-endian numbers in-place: a = (a + b) mod 2^160.
 */
function add160(a: Buffer, b: Buffer): void {
  let carry = 0;
  for (let i = 19; i >= 0; i--) {
    const sum = a[i]! + b[i]! + carry;
    a[i] = sum & 0xff;
    carry = sum >> 8;
  }
}

/**
 * Increment a 20-byte big-endian number in-place: a = (a + 1) mod 2^160.
 */
function increment160(a: Buffer): void {
  for (let i = 19; i >= 0; i--) {
    const val = a[i]! + 1;
    a[i] = val & 0xff;
    if (val < 256) break; // no carry
  }
}

export interface DerivedKeys {
  kEncr: Buffer; // 16 bytes — encryption key
  kAut: Buffer;  // 16 bytes — authentication key
  msk: Buffer;   // 64 bytes — Master Session Key
  emsk: Buffer;  // 64 bytes — Extended MSK
}

/**
 * Derive all session keys from identity, IK, and CK.
 *
 * MK = SHA-1(identity | IK | CK)
 * PRF(MK, 160) → K_encr(16) | K_aut(16) | MSK(64) | EMSK(64)
 */
export function deriveKeys(identity: string, ik: Buffer, ck: Buffer): DerivedKeys {
  const mk = deriveMasterKey(identity, ik, ck);
  const prfOutput = prfSha1(mk, 160);

  return {
    kEncr: prfOutput.subarray(0, 16),
    kAut: prfOutput.subarray(16, 32),
    msk: prfOutput.subarray(32, 96),
    emsk: prfOutput.subarray(96, 160),
  };
}

/**
 * Compute the AT_MAC value for an EAP packet.
 *
 * MAC = HMAC-SHA-1(K_aut, eapPacketWithZeroedMac) truncated to 16 bytes.
 *
 * The caller must ensure the MAC field in the packet bytes is zeroed before calling.
 */
export function computeMac(kAut: Buffer, eapPacketWithZeroMac: Buffer): Buffer {
  const hmac = crypto.createHmac('sha1', kAut);
  hmac.update(eapPacketWithZeroMac);
  return hmac.digest().subarray(0, 16);
}

/**
 * Derive new session keys for EAP-AKA fast re-authentication.
 * Per RFC 4187 Section 7:
 *
 * XKEY' = SHA-1(Identity | counter(2 bytes BE) | NONCE_S | MK)
 * PRF(XKEY', 128) → MSK'(64) | EMSK'(64)
 *
 * K_aut and K_encr are NOT re-derived — they persist from the original full auth.
 */
export function deriveReauthKeys(
  identity: string,
  counter: number,
  nonceS: Buffer,
  mk: Buffer,
): { msk: Buffer; emsk: Buffer } {
  const hash = crypto.createHash('sha1');
  hash.update(Buffer.from(identity, 'utf-8'));
  const counterBuf = Buffer.alloc(2);
  counterBuf.writeUInt16BE(counter, 0);
  hash.update(counterBuf);
  hash.update(nonceS);
  hash.update(mk);
  const xkeyPrime = hash.digest();

  const prfOutput = prfSha1(xkeyPrime, 128);

  return {
    msk: prfOutput.subarray(0, 64),
    emsk: prfOutput.subarray(64, 128),
  };
}

/**
 * Verify the AT_MAC in a received EAP packet.
 *
 * Zeros the MAC field in a copy of the packet bytes, recomputes, and
 * compares with timing-safe equality.
 */
export function verifyMac(
  kAut: Buffer,
  rawPacketBytes: Buffer,
  macOffset: number,
  receivedMac: Buffer,
): boolean {
  // Create a copy with the MAC field zeroed
  const copy = Buffer.from(rawPacketBytes);
  copy.fill(0, macOffset, macOffset + 16);

  const computed = computeMac(kAut, copy);
  return crypto.timingSafeEqual(computed, receivedMac);
}
