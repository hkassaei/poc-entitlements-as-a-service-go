import { describe, it, expect } from 'vitest';
import crypto from 'node:crypto';
import {
  buildIdentity,
  deriveMasterKey,
  prfSha1,
  deriveKeys,
  deriveReauthKeys,
  computeMac,
  verifyMac,
} from '../../src/auth/keyDerivation.js';
import { encodeEapPacket, EAP_CODE, EAP_TYPE_AKA, AKA_SUBTYPE, AT } from '../../src/auth/eapCodec.js';

describe('Key Derivation', () => {
  const testImsi = '001010000000001';
  const testIk = Buffer.alloc(16, 0x11);
  const testCk = Buffer.alloc(16, 0x22);

  describe('buildIdentity', () => {
    it('prepends "0" to IMSI', () => {
      expect(buildIdentity('001010000000001')).toBe('0001010000000001');
    });
  });

  describe('deriveMasterKey', () => {
    it('produces a 20-byte SHA-1 digest', () => {
      const identity = buildIdentity(testImsi);
      const mk = deriveMasterKey(identity, testIk, testCk);
      expect(mk.length).toBe(20);
    });

    it('produces deterministic output', () => {
      const identity = buildIdentity(testImsi);
      const mk1 = deriveMasterKey(identity, testIk, testCk);
      const mk2 = deriveMasterKey(identity, testIk, testCk);
      expect(mk1).toEqual(mk2);
    });

    it('produces correct SHA-1', () => {
      const identity = buildIdentity(testImsi);
      const expected = crypto
        .createHash('sha1')
        .update(Buffer.from(identity, 'utf-8'))
        .update(testIk)
        .update(testCk)
        .digest();
      const mk = deriveMasterKey(identity, testIk, testCk);
      expect(mk).toEqual(expected);
    });
  });

  describe('prfSha1', () => {
    it('produces output of requested length', () => {
      const mk = Buffer.alloc(20, 0x42);
      const output = prfSha1(mk, 160);
      expect(output.length).toBe(160);
    });

    it('produces deterministic output', () => {
      const mk = Buffer.alloc(20, 0x42);
      const out1 = prfSha1(mk, 160);
      const out2 = prfSha1(mk, 160);
      expect(out1).toEqual(out2);
    });

    it('handles partial block at end', () => {
      const mk = Buffer.alloc(20, 0x42);
      const output = prfSha1(mk, 10);
      expect(output.length).toBe(10);
    });
  });

  describe('deriveKeys', () => {
    it('splits PRF output into correct key sizes', () => {
      const identity = buildIdentity(testImsi);
      const keys = deriveKeys(identity, testIk, testCk);

      expect(keys.kEncr.length).toBe(16);
      expect(keys.kAut.length).toBe(16);
      expect(keys.msk.length).toBe(64);
      expect(keys.emsk.length).toBe(64);
    });

    it('produces deterministic keys', () => {
      const identity = buildIdentity(testImsi);
      const keys1 = deriveKeys(identity, testIk, testCk);
      const keys2 = deriveKeys(identity, testIk, testCk);

      expect(keys1.kEncr).toEqual(keys2.kEncr);
      expect(keys1.kAut).toEqual(keys2.kAut);
      expect(keys1.msk).toEqual(keys2.msk);
      expect(keys1.emsk).toEqual(keys2.emsk);
    });

    it('produces different keys for different inputs', () => {
      const identity = buildIdentity(testImsi);
      const keys1 = deriveKeys(identity, testIk, testCk);

      const differentCk = Buffer.alloc(16, 0x33);
      const keys2 = deriveKeys(identity, testIk, differentCk);

      expect(keys1.kAut).not.toEqual(keys2.kAut);
    });
  });

  describe('deriveReauthKeys', () => {
    const mk = Buffer.alloc(20, 0x42);
    const nonceS = crypto.randomBytes(16);
    const identity = 'test-reauth-identity';

    it('produces MSK of 64 bytes and EMSK of 64 bytes', () => {
      const keys = deriveReauthKeys(identity, 1, nonceS, mk);
      expect(keys.msk.length).toBe(64);
      expect(keys.emsk.length).toBe(64);
    });

    it('produces deterministic output', () => {
      const keys1 = deriveReauthKeys(identity, 1, nonceS, mk);
      const keys2 = deriveReauthKeys(identity, 1, nonceS, mk);
      expect(keys1.msk).toEqual(keys2.msk);
      expect(keys1.emsk).toEqual(keys2.emsk);
    });

    it('produces different keys for different counters', () => {
      const keys1 = deriveReauthKeys(identity, 1, nonceS, mk);
      const keys2 = deriveReauthKeys(identity, 2, nonceS, mk);
      expect(keys1.msk).not.toEqual(keys2.msk);
    });

    it('produces different keys for different nonces', () => {
      const nonceS2 = crypto.randomBytes(16);
      const keys1 = deriveReauthKeys(identity, 1, nonceS, mk);
      const keys2 = deriveReauthKeys(identity, 1, nonceS2, mk);
      expect(keys1.msk).not.toEqual(keys2.msk);
    });

    it('produces different keys for different identities', () => {
      const keys1 = deriveReauthKeys(identity, 1, nonceS, mk);
      const keys2 = deriveReauthKeys('different-identity', 1, nonceS, mk);
      expect(keys1.msk).not.toEqual(keys2.msk);
    });

    it('produces different keys for different MKs', () => {
      const mk2 = Buffer.alloc(20, 0x99);
      const keys1 = deriveReauthKeys(identity, 1, nonceS, mk);
      const keys2 = deriveReauthKeys(identity, 1, nonceS, mk2);
      expect(keys1.msk).not.toEqual(keys2.msk);
    });
  });

  describe('computeMac / verifyMac', () => {
    it('produces a 16-byte MAC', () => {
      const kAut = Buffer.alloc(16, 0xaa);
      const data = Buffer.alloc(64, 0xbb);
      const mac = computeMac(kAut, data);
      expect(mac.length).toBe(16);
    });

    it('round-trips with verifyMac', () => {
      const identity = buildIdentity(testImsi);
      const keys = deriveKeys(identity, testIk, testCk);

      // Build a challenge packet with zeroed MAC
      const rand = crypto.randomBytes(16);
      const autn = crypto.randomBytes(16);
      const packet = encodeEapPacket({
        code: EAP_CODE.REQUEST,
        identifier: 1,
        type: EAP_TYPE_AKA,
        subtype: AKA_SUBTYPE.CHALLENGE,
        attributes: [
          { type: AT.AT_RAND, value: rand },
          { type: AT.AT_AUTN, value: autn },
          { type: AT.AT_MAC, value: Buffer.alloc(16) }, // zeroed MAC
        ],
      });

      // Compute MAC over packet with zeroed MAC field
      const mac = computeMac(keys.kAut, packet);

      // Patch the MAC into the packet
      // AT_MAC starts at offset: 8 (header) + 20 (AT_RAND) + 20 (AT_AUTN) = 48
      // MAC value is at offset 48 + 4 (type+length+reserved) = 52
      mac.copy(packet, 52);

      // Verify should pass
      expect(verifyMac(keys.kAut, packet, 52, mac)).toBe(true);
    });

    it('rejects tampered packets', () => {
      const identity = buildIdentity(testImsi);
      const keys = deriveKeys(identity, testIk, testCk);

      const packet = encodeEapPacket({
        code: EAP_CODE.REQUEST,
        identifier: 1,
        type: EAP_TYPE_AKA,
        subtype: AKA_SUBTYPE.CHALLENGE,
        attributes: [
          { type: AT.AT_RAND, value: crypto.randomBytes(16) },
          { type: AT.AT_AUTN, value: crypto.randomBytes(16) },
          { type: AT.AT_MAC, value: Buffer.alloc(16) },
        ],
      });

      const mac = computeMac(keys.kAut, packet);
      mac.copy(packet, 52);

      // Tamper with the RAND value
      packet[10] = (packet[10]! + 1) & 0xff;

      // Verify should fail
      expect(verifyMac(keys.kAut, packet, 52, mac)).toBe(false);
    });
  });
});
