import { describe, it, expect } from 'vitest';
import crypto from 'node:crypto';
import { encryptAttributes, decryptAttributes } from '../../src/auth/eapEncryption.js';
import { AT, type EapAttribute } from '../../src/auth/eapCodec.js';

describe('EAP Encryption', () => {
  const kEncr = crypto.randomBytes(16);
  const iv = crypto.randomBytes(16);

  describe('encryptAttributes / decryptAttributes', () => {
    it('round-trips AT_COUNTER', () => {
      const counterBuf = Buffer.alloc(2);
      counterBuf.writeUInt16BE(42, 0);

      const innerAttrs: EapAttribute[] = [
        { type: AT.AT_COUNTER, value: counterBuf },
      ];

      const ciphertext = encryptAttributes(kEncr, iv, innerAttrs);
      expect(ciphertext.length).toBeGreaterThan(0);
      expect(ciphertext.length % 16).toBe(0); // AES block-aligned

      const decrypted = decryptAttributes(kEncr, iv, ciphertext);
      expect(decrypted).toHaveLength(1);
      expect(decrypted[0]!.type).toBe(AT.AT_COUNTER);
      expect(decrypted[0]!.value.readUInt16BE(0)).toBe(42);
    });

    it('round-trips AT_COUNTER + AT_NONCE_S + AT_NEXT_REAUTH_ID', () => {
      const counterBuf = Buffer.alloc(2);
      counterBuf.writeUInt16BE(7, 0);
      const nonceS = crypto.randomBytes(16);
      const nextReauthId = 'test-reauth-identity-123';

      const innerAttrs: EapAttribute[] = [
        { type: AT.AT_COUNTER, value: counterBuf },
        { type: AT.AT_NONCE_S, value: nonceS },
        { type: AT.AT_NEXT_REAUTH_ID, value: Buffer.from(nextReauthId, 'utf-8') },
      ];

      const ciphertext = encryptAttributes(kEncr, iv, innerAttrs);
      const decrypted = decryptAttributes(kEncr, iv, ciphertext);

      expect(decrypted).toHaveLength(3);

      expect(decrypted[0]!.type).toBe(AT.AT_COUNTER);
      expect(decrypted[0]!.value.readUInt16BE(0)).toBe(7);

      expect(decrypted[1]!.type).toBe(AT.AT_NONCE_S);
      expect(decrypted[1]!.value).toEqual(nonceS);

      expect(decrypted[2]!.type).toBe(AT.AT_NEXT_REAUTH_ID);
      expect(decrypted[2]!.value.toString('utf-8')).toBe(nextReauthId);
    });

    it('produces different ciphertext with different IVs', () => {
      const counterBuf = Buffer.alloc(2);
      counterBuf.writeUInt16BE(1, 0);
      const innerAttrs: EapAttribute[] = [
        { type: AT.AT_COUNTER, value: counterBuf },
      ];

      const iv2 = crypto.randomBytes(16);
      const ct1 = encryptAttributes(kEncr, iv, innerAttrs);
      const ct2 = encryptAttributes(kEncr, iv2, innerAttrs);

      expect(ct1).not.toEqual(ct2);
    });

    it('produces different ciphertext with different keys', () => {
      const counterBuf = Buffer.alloc(2);
      counterBuf.writeUInt16BE(1, 0);
      const innerAttrs: EapAttribute[] = [
        { type: AT.AT_COUNTER, value: counterBuf },
      ];

      const kEncr2 = crypto.randomBytes(16);
      const ct1 = encryptAttributes(kEncr, iv, innerAttrs);
      const ct2 = encryptAttributes(kEncr2, iv, innerAttrs);

      expect(ct1).not.toEqual(ct2);
    });

    it('ciphertext length is always a multiple of 16', () => {
      const nextReauthId = 'short';
      const innerAttrs: EapAttribute[] = [
        { type: AT.AT_NEXT_REAUTH_ID, value: Buffer.from(nextReauthId, 'utf-8') },
      ];

      const ciphertext = encryptAttributes(kEncr, iv, innerAttrs);
      expect(ciphertext.length % 16).toBe(0);
    });

    it('strips AT_PADDING from decrypted result', () => {
      // An identity that forces padding
      const nextReauthId = 'a';
      const innerAttrs: EapAttribute[] = [
        { type: AT.AT_NEXT_REAUTH_ID, value: Buffer.from(nextReauthId, 'utf-8') },
      ];

      const ciphertext = encryptAttributes(kEncr, iv, innerAttrs);
      const decrypted = decryptAttributes(kEncr, iv, ciphertext);

      // Should not contain AT_PADDING
      const hasPadding = decrypted.some((a) => a.type === AT.AT_PADDING);
      expect(hasPadding).toBe(false);

      expect(decrypted).toHaveLength(1);
      expect(decrypted[0]!.type).toBe(AT.AT_NEXT_REAUTH_ID);
      expect(decrypted[0]!.value.toString('utf-8')).toBe('a');
    });

    it('round-trips AT_COUNTER_TOO_SMALL', () => {
      const counterBuf = Buffer.alloc(2);
      counterBuf.writeUInt16BE(5, 0);

      const innerAttrs: EapAttribute[] = [
        { type: AT.AT_COUNTER, value: counterBuf },
        { type: AT.AT_COUNTER_TOO_SMALL, value: Buffer.alloc(0) },
      ];

      const ciphertext = encryptAttributes(kEncr, iv, innerAttrs);
      const decrypted = decryptAttributes(kEncr, iv, ciphertext);

      expect(decrypted).toHaveLength(2);
      expect(decrypted[0]!.type).toBe(AT.AT_COUNTER);
      expect(decrypted[1]!.type).toBe(AT.AT_COUNTER_TOO_SMALL);
    });
  });
});
