import { describe, it, expect } from 'vitest';
import { computeOPc, generateVectorsWithRand } from '../src/milenage.js';

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
