import { describe, it, expect } from 'vitest';
import { computeOPc, generateVectorsWithRand, f5Star, f1Star, validateAuts, generateAuts } from '../src/milenage.js';

// 3GPP TS 35.207 Test Set 1
describe('MILENAGE Test Set 1 (TS 35.207)', () => {
  const ki = Buffer.from('465b5ce8b199b49faa5f0a2ee238a6bc', 'hex');
  const op = Buffer.from('cdc202d5123e20f62b6d676ac72cb318', 'hex');
  const rand = Buffer.from('23553cbe9637a89d218ae64dae47bf35', 'hex');
  const sqn = Buffer.from('ff9bb4d0b607', 'hex');
  const amf = Buffer.from('b9b9', 'hex');

  it('computes correct OPc', () => {
    const opc = computeOPc(ki, op);
    expect(opc.toString('hex')).toBe('cd63cb71954a9f4e48a5994e37a02baf');
  });

  it('generates correct f1 (MAC-A)', () => {
    const vectors = generateVectorsWithRand(ki, op, rand, sqn, amf);
    // AUTN = (SQN^AK) || AMF || MAC-A, so MAC-A = AUTN[8..15]
    const macA = vectors.autn.subarray(8, 16).toString('hex');
    expect(macA).toBe('4a9ffac354dfafb3');
  });

  it('generates correct f2 (RES/XRES)', () => {
    const vectors = generateVectorsWithRand(ki, op, rand, sqn, amf);
    expect(vectors.xres.toString('hex')).toBe('a54211d5e3ba50bf');
  });

  it('generates correct f3 (CK)', () => {
    const vectors = generateVectorsWithRand(ki, op, rand, sqn, amf);
    expect(vectors.ck.toString('hex')).toBe('b40ba9a3c58b2a05bbf0d987b21bf8cb');
  });

  it('generates correct f4 (IK)', () => {
    const vectors = generateVectorsWithRand(ki, op, rand, sqn, amf);
    expect(vectors.ik.toString('hex')).toBe('f769bcd751044604127672711c6d3441');
  });

  it('generates correct f5 (AK)', () => {
    const vectors = generateVectorsWithRand(ki, op, rand, sqn, amf);
    expect(vectors.ak.toString('hex')).toBe('aa689c648370');
  });
});

// 3GPP TS 35.207 Test Set 2
describe('MILENAGE Test Set 2 (TS 35.207)', () => {
  const ki = Buffer.from('0396eb317b6d1c36f19c1c84cd6ffd16', 'hex');
  const op = Buffer.from('ff53bade17df5d4e793073ce9d7579fa', 'hex');
  const rand = Buffer.from('c00d603103dcee52c4478119494202e8', 'hex');
  const sqn = Buffer.from('fd8eef40df7d', 'hex');
  const amf = Buffer.from('af17', 'hex');

  it('computes correct OPc', () => {
    const opc = computeOPc(ki, op);
    expect(opc.toString('hex')).toBe('53c15671c60a4b731c55b4a441c0bde2');
  });

  it('generates correct f1 (MAC-A)', () => {
    const vectors = generateVectorsWithRand(ki, op, rand, sqn, amf);
    const macA = vectors.autn.subarray(8, 16).toString('hex');
    expect(macA).toBe('5df5b31807e258b0');
  });

  it('generates correct f2 (RES/XRES)', () => {
    const vectors = generateVectorsWithRand(ki, op, rand, sqn, amf);
    expect(vectors.xres.toString('hex')).toBe('d3a628ed988620f0');
  });

  it('generates correct f3 (CK)', () => {
    const vectors = generateVectorsWithRand(ki, op, rand, sqn, amf);
    expect(vectors.ck.toString('hex')).toBe('58c433ff7a7082acd424220f2b67c556');
  });

  it('generates correct f4 (IK)', () => {
    const vectors = generateVectorsWithRand(ki, op, rand, sqn, amf);
    expect(vectors.ik.toString('hex')).toBe('21a8c1f929702adb3e738488b9f5c5da');
  });

  it('generates correct f5 (AK)', () => {
    const vectors = generateVectorsWithRand(ki, op, rand, sqn, amf);
    expect(vectors.ak.toString('hex')).toBe('c47783995f72');
  });
});

// f5* and f1* tests (resync functions)
describe('MILENAGE Resync Functions (f5*, f1*)', () => {
  // Using Test Set 1 for deterministic values
  const ki = Buffer.from('465b5ce8b199b49faa5f0a2ee238a6bc', 'hex');
  const op = Buffer.from('cdc202d5123e20f62b6d676ac72cb318', 'hex');
  const rand = Buffer.from('23553cbe9637a89d218ae64dae47bf35', 'hex');

  describe('f5* (Anonymity Key for Resync)', () => {
    it('produces a 6-byte AK*', () => {
      const akStar = f5Star(ki, rand, op);
      expect(akStar.length).toBe(6);
    });

    it('produces AK* different from f5 AK (different constants)', () => {
      const vectors = generateVectorsWithRand(ki, op, rand, Buffer.from('ff9bb4d0b607', 'hex'), Buffer.from('b9b9', 'hex'));
      const ak = vectors.ak;
      const akStar = f5Star(ki, rand, op);
      expect(akStar.toString('hex')).not.toBe(ak.toString('hex'));
    });

    it('is deterministic for the same inputs', () => {
      const akStar1 = f5Star(ki, rand, op);
      const akStar2 = f5Star(ki, rand, op);
      expect(akStar1.toString('hex')).toBe(akStar2.toString('hex'));
    });
  });

  describe('f1* (MAC-S for Resync)', () => {
    it('produces an 8-byte MAC-S', () => {
      const sqnMs = Buffer.from('000000000100', 'hex');
      const amfZero = Buffer.alloc(2, 0);
      const macS = f1Star(ki, rand, sqnMs, amfZero, op);
      expect(macS.length).toBe(8);
    });

    it('produces MAC-S different from f1 MAC-A (different constants)', () => {
      const sqn = Buffer.from('ff9bb4d0b607', 'hex');
      const amf = Buffer.from('b9b9', 'hex');
      const amfZero = Buffer.alloc(2, 0);

      const vectors = generateVectorsWithRand(ki, op, rand, sqn, amf);
      const macA = vectors.autn.subarray(8, 16);
      const macS = f1Star(ki, rand, sqn, amfZero, op);

      // MAC-A uses different AMF and different constant C1, so they differ
      expect(macS.toString('hex')).not.toBe(macA.toString('hex'));
    });

    it('is deterministic for the same inputs', () => {
      const sqnMs = Buffer.from('000000000200', 'hex');
      const amfZero = Buffer.alloc(2, 0);
      const macS1 = f1Star(ki, rand, sqnMs, amfZero, op);
      const macS2 = f1Star(ki, rand, sqnMs, amfZero, op);
      expect(macS1.toString('hex')).toBe(macS2.toString('hex'));
    });
  });

  describe('validateAuts', () => {
    it('returns valid=true for correct AUTS', () => {
      const sqnMs = Buffer.from('000000000100', 'hex');
      const auts = generateAuts(ki, rand, sqnMs, op);
      const result = validateAuts(ki, rand, auts, op);
      expect(result.valid).toBe(true);
      expect(result.sqnMs).toBeDefined();
      expect(result.sqnMs!.toString('hex')).toBe(sqnMs.toString('hex'));
    });

    it('returns valid=false for tampered AUTS (modified MAC-S)', () => {
      const sqnMs = Buffer.from('000000000100', 'hex');
      const auts = generateAuts(ki, rand, sqnMs, op);
      // Tamper with the MAC-S (bytes 6-13)
      auts[10] ^= 0xff;
      const result = validateAuts(ki, rand, auts, op);
      expect(result.valid).toBe(false);
      expect(result.sqnMs).toBeUndefined();
    });

    it('returns valid=false for tampered AUTS (modified concealed-SQN)', () => {
      const sqnMs = Buffer.from('000000000100', 'hex');
      const auts = generateAuts(ki, rand, sqnMs, op);
      // Tamper with the concealed-SQN (bytes 0-5)
      auts[3] ^= 0xff;
      const result = validateAuts(ki, rand, auts, op);
      expect(result.valid).toBe(false);
    });

    it('returns valid=false for wrong RAND', () => {
      const sqnMs = Buffer.from('000000000100', 'hex');
      const auts = generateAuts(ki, rand, sqnMs, op);
      const wrongRand = Buffer.from('0000000000000000000000000000dead', 'hex');
      const result = validateAuts(ki, wrongRand, auts, op);
      expect(result.valid).toBe(false);
    });

    it('returns valid=false for wrong Ki', () => {
      const sqnMs = Buffer.from('000000000100', 'hex');
      const auts = generateAuts(ki, rand, sqnMs, op);
      const wrongKi = Buffer.from('deadbeefdeadbeefdeadbeefdeadbeef', 'hex');
      const result = validateAuts(wrongKi, rand, auts, op);
      expect(result.valid).toBe(false);
    });

    it('returns valid=false for invalid AUTS length', () => {
      const result = validateAuts(ki, rand, Buffer.from('tooshort', 'utf8'), op);
      expect(result.valid).toBe(false);
    });

    it('round-trip: generate AUTS then validate extracts same SQN_MS', () => {
      // Test with various SQN values
      const testSqns = [
        Buffer.from('000000000000', 'hex'),
        Buffer.from('000000000001', 'hex'),
        Buffer.from('0000000000ff', 'hex'),
        Buffer.from('ffffffffffff', 'hex'),
        Buffer.from('123456789abc', 'hex'),
      ];

      for (const sqnMs of testSqns) {
        const auts = generateAuts(ki, rand, sqnMs, op);
        const result = validateAuts(ki, rand, auts, op);
        expect(result.valid).toBe(true);
        expect(result.sqnMs!.toString('hex')).toBe(sqnMs.toString('hex'));
      }
    });
  });

  describe('generateAuts', () => {
    it('produces a 14-byte AUTS', () => {
      const sqnMs = Buffer.from('000000000100', 'hex');
      const auts = generateAuts(ki, rand, sqnMs, op);
      expect(auts.length).toBe(14);
    });

    it('AUTS structure: first 6 bytes concealed-SQN, next 8 bytes MAC-S', () => {
      const sqnMs = Buffer.from('abcdef012345', 'hex');
      const auts = generateAuts(ki, rand, sqnMs, op);

      // Extract parts
      const concealedSqn = auts.subarray(0, 6);
      const macS = auts.subarray(6, 14);

      // Verify concealed-SQN = SQN_MS XOR AK*
      const akStar = f5Star(ki, rand, op);
      const expectedConcealedSqn = Buffer.alloc(6);
      for (let i = 0; i < 6; i++) {
        expectedConcealedSqn[i] = sqnMs[i]! ^ akStar[i]!;
      }
      expect(concealedSqn.toString('hex')).toBe(expectedConcealedSqn.toString('hex'));

      // Verify MAC-S
      const amfZero = Buffer.alloc(2, 0);
      const expectedMacS = f1Star(ki, rand, sqnMs, amfZero, op);
      expect(macS.toString('hex')).toBe(expectedMacS.toString('hex'));
    });
  });
});
