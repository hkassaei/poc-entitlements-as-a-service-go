import { describe, it, expect } from 'vitest';
import {
  encodeEapPacket,
  decodeEapPacket,
  encodeEapToBase64,
  decodeEapFromBase64,
  EAP_CODE,
  EAP_TYPE_AKA,
  AKA_SUBTYPE,
  AT,
  type EapPacket,
} from '../../src/auth/eapCodec.js';

describe('EAP Codec', () => {
  describe('EAP-Success / EAP-Failure', () => {
    it('encodes EAP-Success to 4 bytes', () => {
      const packet: EapPacket = { code: EAP_CODE.SUCCESS, identifier: 42 };
      const buf = encodeEapPacket(packet);
      expect(buf.length).toBe(4);
      expect(buf[0]).toBe(EAP_CODE.SUCCESS);
      expect(buf[1]).toBe(42);
      expect(buf.readUInt16BE(2)).toBe(4);
    });

    it('encodes EAP-Failure to 4 bytes', () => {
      const packet: EapPacket = { code: EAP_CODE.FAILURE, identifier: 7 };
      const buf = encodeEapPacket(packet);
      expect(buf.length).toBe(4);
      expect(buf[0]).toBe(EAP_CODE.FAILURE);
      expect(buf[1]).toBe(7);
      expect(buf.readUInt16BE(2)).toBe(4);
    });

    it('round-trips EAP-Success', () => {
      const original: EapPacket = { code: EAP_CODE.SUCCESS, identifier: 99 };
      const decoded = decodeEapPacket(encodeEapPacket(original));
      expect(decoded.code).toBe(EAP_CODE.SUCCESS);
      expect(decoded.identifier).toBe(99);
    });

    it('round-trips EAP-Failure', () => {
      const original: EapPacket = { code: EAP_CODE.FAILURE, identifier: 1 };
      const decoded = decodeEapPacket(encodeEapPacket(original));
      expect(decoded.code).toBe(EAP_CODE.FAILURE);
      expect(decoded.identifier).toBe(1);
    });
  });

  describe('AKA-Challenge (Request with AT_RAND + AT_AUTN + AT_MAC)', () => {
    const rand = Buffer.alloc(16, 0xaa);
    const autn = Buffer.alloc(16, 0xbb);
    const mac = Buffer.alloc(16, 0xcc);

    const challengePacket: EapPacket = {
      code: EAP_CODE.REQUEST,
      identifier: 10,
      type: EAP_TYPE_AKA,
      subtype: AKA_SUBTYPE.CHALLENGE,
      attributes: [
        { type: AT.AT_RAND, value: rand },
        { type: AT.AT_AUTN, value: autn },
        { type: AT.AT_MAC, value: mac },
      ],
    };

    it('encodes to correct total length', () => {
      const buf = encodeEapPacket(challengePacket);
      // Header(8) + AT_RAND(20) + AT_AUTN(20) + AT_MAC(20) = 68
      expect(buf.length).toBe(68);
      expect(buf.readUInt16BE(2)).toBe(68);
    });

    it('round-trips all attributes', () => {
      const buf = encodeEapPacket(challengePacket);
      const decoded = decodeEapPacket(buf);

      expect(decoded.code).toBe(EAP_CODE.REQUEST);
      expect(decoded.identifier).toBe(10);
      expect(decoded.type).toBe(EAP_TYPE_AKA);
      expect(decoded.subtype).toBe(AKA_SUBTYPE.CHALLENGE);
      expect(decoded.attributes).toHaveLength(3);

      const decodedRand = decoded.attributes!.find((a) => a.type === AT.AT_RAND)!;
      const decodedAutn = decoded.attributes!.find((a) => a.type === AT.AT_AUTN)!;
      const decodedMac = decoded.attributes!.find((a) => a.type === AT.AT_MAC)!;

      expect(decodedRand.value).toEqual(rand);
      expect(decodedAutn.value).toEqual(autn);
      expect(decodedMac.value).toEqual(mac);
    });
  });

  describe('AKA-Response (Response with AT_RES + AT_MAC)', () => {
    const xres = Buffer.alloc(8, 0xdd); // 8 bytes = 64 bits
    const mac = Buffer.alloc(16, 0xee);

    const responsePacket: EapPacket = {
      code: EAP_CODE.RESPONSE,
      identifier: 10,
      type: EAP_TYPE_AKA,
      subtype: AKA_SUBTYPE.CHALLENGE,
      attributes: [
        { type: AT.AT_RES, value: xres },
        { type: AT.AT_MAC, value: mac },
      ],
    };

    it('encodes AT_RES with correct bit-length prefix', () => {
      const buf = encodeEapPacket(responsePacket);
      // AT_RES: type(1) + length(1) + bitlen(2) + value(8) = 12, padded to 12 (already aligned)
      // Header(8) + AT_RES(12) + AT_MAC(20) = 40
      expect(buf.length).toBe(40);
    });

    it('round-trips AT_RES value', () => {
      const buf = encodeEapPacket(responsePacket);
      const decoded = decodeEapPacket(buf);

      const decodedRes = decoded.attributes!.find((a) => a.type === AT.AT_RES)!;
      expect(decodedRes.value).toEqual(xres);
      expect(decodedRes.value.length).toBe(8);
    });

    it('round-trips AT_MAC value', () => {
      const buf = encodeEapPacket(responsePacket);
      const decoded = decodeEapPacket(buf);

      const decodedMac = decoded.attributes!.find((a) => a.type === AT.AT_MAC)!;
      expect(decodedMac.value).toEqual(mac);
    });
  });

  describe('Base64 encoding/decoding', () => {
    it('round-trips through base64', () => {
      const packet: EapPacket = {
        code: EAP_CODE.REQUEST,
        identifier: 5,
        type: EAP_TYPE_AKA,
        subtype: AKA_SUBTYPE.IDENTITY,
        attributes: [],
      };

      const b64 = encodeEapToBase64(packet);
      expect(typeof b64).toBe('string');

      const decoded = decodeEapFromBase64(b64);
      expect(decoded.code).toBe(EAP_CODE.REQUEST);
      expect(decoded.identifier).toBe(5);
      expect(decoded.subtype).toBe(AKA_SUBTYPE.IDENTITY);
    });
  });

  describe('AKA-Reauthentication (AT_IV + AT_ENCR_DATA + AT_MAC)', () => {
    const iv = Buffer.alloc(16, 0x11);
    const ciphertext = Buffer.alloc(32, 0x22); // 2 blocks of AES
    const mac = Buffer.alloc(16, 0x33);

    const reauthPacket: EapPacket = {
      code: EAP_CODE.REQUEST,
      identifier: 20,
      type: EAP_TYPE_AKA,
      subtype: AKA_SUBTYPE.REAUTHENTICATION,
      attributes: [
        { type: AT.AT_IV, value: iv },
        { type: AT.AT_ENCR_DATA, value: ciphertext },
        { type: AT.AT_MAC, value: mac },
      ],
    };

    it('round-trips AT_IV', () => {
      const buf = encodeEapPacket(reauthPacket);
      const decoded = decodeEapPacket(buf);
      const decodedIv = decoded.attributes!.find((a) => a.type === AT.AT_IV)!;
      expect(decodedIv.value).toEqual(iv);
      expect(decodedIv.value.length).toBe(16);
    });

    it('round-trips AT_ENCR_DATA', () => {
      const buf = encodeEapPacket(reauthPacket);
      const decoded = decodeEapPacket(buf);
      const decodedEncr = decoded.attributes!.find((a) => a.type === AT.AT_ENCR_DATA)!;
      expect(decodedEncr.value).toEqual(ciphertext);
    });

    it('round-trips AT_MAC in reauthentication packet', () => {
      const buf = encodeEapPacket(reauthPacket);
      const decoded = decodeEapPacket(buf);
      const decodedMac = decoded.attributes!.find((a) => a.type === AT.AT_MAC)!;
      expect(decodedMac.value).toEqual(mac);
    });

    it('uses REAUTHENTICATION subtype (13)', () => {
      const buf = encodeEapPacket(reauthPacket);
      const decoded = decodeEapPacket(buf);
      expect(decoded.subtype).toBe(AKA_SUBTYPE.REAUTHENTICATION);
      expect(decoded.subtype).toBe(13);
    });
  });

  describe('AT_COUNTER', () => {
    it('round-trips counter value', () => {
      const counterBuf = Buffer.alloc(2);
      counterBuf.writeUInt16BE(1234, 0);

      const packet: EapPacket = {
        code: EAP_CODE.RESPONSE,
        identifier: 5,
        type: EAP_TYPE_AKA,
        subtype: AKA_SUBTYPE.REAUTHENTICATION,
        attributes: [
          { type: AT.AT_COUNTER, value: counterBuf },
        ],
      };

      const buf = encodeEapPacket(packet);
      const decoded = decodeEapPacket(buf);
      const decodedCounter = decoded.attributes!.find((a) => a.type === AT.AT_COUNTER)!;
      expect(decodedCounter.value.readUInt16BE(0)).toBe(1234);
    });

    it('encodes AT_COUNTER to 4 bytes total (length=1)', () => {
      const counterBuf = Buffer.alloc(2);
      counterBuf.writeUInt16BE(0, 0);

      const packet: EapPacket = {
        code: EAP_CODE.RESPONSE,
        identifier: 1,
        type: EAP_TYPE_AKA,
        subtype: AKA_SUBTYPE.REAUTHENTICATION,
        attributes: [
          { type: AT.AT_COUNTER, value: counterBuf },
        ],
      };

      const buf = encodeEapPacket(packet);
      // Header(8) + AT_COUNTER(4) = 12
      expect(buf.length).toBe(12);
    });
  });

  describe('AT_COUNTER_TOO_SMALL', () => {
    it('round-trips (empty value)', () => {
      const packet: EapPacket = {
        code: EAP_CODE.RESPONSE,
        identifier: 5,
        type: EAP_TYPE_AKA,
        subtype: AKA_SUBTYPE.REAUTHENTICATION,
        attributes: [
          { type: AT.AT_COUNTER_TOO_SMALL, value: Buffer.alloc(0) },
        ],
      };

      const buf = encodeEapPacket(packet);
      const decoded = decodeEapPacket(buf);
      const decodedAttr = decoded.attributes!.find((a) => a.type === AT.AT_COUNTER_TOO_SMALL)!;
      expect(decodedAttr).toBeDefined();
      expect(decodedAttr.value.length).toBe(0);
    });
  });

  describe('AT_NONCE_S', () => {
    it('round-trips 16-byte nonce', () => {
      const nonceS = Buffer.alloc(16, 0x55);

      const packet: EapPacket = {
        code: EAP_CODE.REQUEST,
        identifier: 7,
        type: EAP_TYPE_AKA,
        subtype: AKA_SUBTYPE.REAUTHENTICATION,
        attributes: [
          { type: AT.AT_NONCE_S, value: nonceS },
        ],
      };

      const buf = encodeEapPacket(packet);
      const decoded = decodeEapPacket(buf);
      const decodedNonce = decoded.attributes!.find((a) => a.type === AT.AT_NONCE_S)!;
      expect(decodedNonce.value).toEqual(nonceS);
      expect(decodedNonce.value.length).toBe(16);
    });
  });

  describe('AT_NEXT_REAUTH_ID', () => {
    it('round-trips UTF-8 identity string', () => {
      const identity = 'test-reauth-id-abc123';
      const identityBuf = Buffer.from(identity, 'utf-8');

      const packet: EapPacket = {
        code: EAP_CODE.REQUEST,
        identifier: 8,
        type: EAP_TYPE_AKA,
        subtype: AKA_SUBTYPE.REAUTHENTICATION,
        attributes: [
          { type: AT.AT_NEXT_REAUTH_ID, value: identityBuf },
        ],
      };

      const buf = encodeEapPacket(packet);
      const decoded = decodeEapPacket(buf);
      const decodedId = decoded.attributes!.find((a) => a.type === AT.AT_NEXT_REAUTH_ID)!;
      expect(decodedId.value.toString('utf-8')).toBe(identity);
    });

    it('handles short identity with padding correctly', () => {
      const identity = 'a';
      const identityBuf = Buffer.from(identity, 'utf-8');

      const packet: EapPacket = {
        code: EAP_CODE.REQUEST,
        identifier: 9,
        type: EAP_TYPE_AKA,
        subtype: AKA_SUBTYPE.REAUTHENTICATION,
        attributes: [
          { type: AT.AT_NEXT_REAUTH_ID, value: identityBuf },
        ],
      };

      const buf = encodeEapPacket(packet);
      const decoded = decodeEapPacket(buf);
      const decodedId = decoded.attributes!.find((a) => a.type === AT.AT_NEXT_REAUTH_ID)!;
      expect(decodedId.value.toString('utf-8')).toBe('a');
    });
  });

  describe('Malformed packets', () => {
    it('rejects packets shorter than 4 bytes', () => {
      expect(() => decodeEapPacket(Buffer.alloc(2))).toThrow('too short');
    });

    it('rejects truncated packets', () => {
      const buf = Buffer.alloc(4);
      buf.writeUInt8(EAP_CODE.REQUEST, 0);
      buf.writeUInt8(1, 1);
      buf.writeUInt16BE(20, 2); // claims 20 bytes but only 4
      expect(() => decodeEapPacket(buf)).toThrow();
    });
  });
});
