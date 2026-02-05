import crypto from 'node:crypto';

// MILENAGE constants (3GPP TS 35.206 §4.1)
const C1 = Buffer.alloc(16, 0); // 0x00...00
const C2 = Buffer.alloc(16, 0); C2[15] = 0x01;
const C3 = Buffer.alloc(16, 0); C3[15] = 0x02;
const C4 = Buffer.alloc(16, 0); C4[15] = 0x04;
const C5 = Buffer.alloc(16, 0); C5[15] = 0x08;

// Resync constants (3GPP TS 35.206 §4.1)
// C1* and C5* are different from C1 and C5 — these are for f1* and f5* specifically
const C1_STAR = Buffer.alloc(16, 0); C1_STAR[15] = 0x80;
const C5_STAR = Buffer.alloc(16, 0); C5_STAR[15] = 0x10;

const R1 = 64;
const R2 = 0;
const R3 = 32;
const R4 = 64;
const R5 = 96;

// R1* = 64 (same as R1), R5* = 0 (NOT 96)
const R1_STAR = 64;
const R5_STAR = 0;

/** Single AES-128-ECB block encryption (16 bytes in, 16 bytes out) */
export function aesEncrypt(key: Buffer, input: Buffer): Buffer {
  const cipher = crypto.createCipheriv('aes-128-ecb', key, null);
  cipher.setAutoPadding(false);
  return Buffer.concat([cipher.update(input), cipher.final()]);
}

/** XOR two equal-length buffers */
export function xor(a: Buffer, b: Buffer): Buffer {
  const result = Buffer.allocUnsafe(a.length);
  for (let i = 0; i < a.length; i++) {
    result[i] = a[i]! ^ b[i]!;
  }
  return result;
}

/** Circular left rotation of a 128-bit (16-byte) buffer by `bits` bits */
export function rotate(buf: Buffer, bits: number): Buffer {
  const byteShift = (bits >>> 3) % 16;
  const bitShift = bits & 7;
  const result = Buffer.allocUnsafe(16);

  for (let i = 0; i < 16; i++) {
    const srcIdx = (i + byteShift) % 16;
    const nextIdx = (i + byteShift + 1) % 16;
    if (bitShift === 0) {
      result[i] = buf[srcIdx]!;
    } else {
      result[i] = ((buf[srcIdx]! << bitShift) | (buf[nextIdx]! >>> (8 - bitShift))) & 0xff;
    }
  }
  return result;
}

/** Compute OPc = AES_K(OP) XOR OP */
export function computeOPc(ki: Buffer, op: Buffer): Buffer {
  return xor(aesEncrypt(ki, op), op);
}

export interface AuthVectors {
  rand: Buffer;
  autn: Buffer;
  xres: Buffer;
  ck: Buffer;
  ik: Buffer;
  ak: Buffer;
}

/**
 * Generate authentication vectors with a random RAND.
 * @param ki  Subscriber key (16 bytes)
 * @param op  Operator variant (16 bytes)
 * @param sqn Sequence number (6 bytes)
 * @param amf Authentication Management Field (2 bytes)
 */
export function generateVectors(
  ki: Buffer,
  op: Buffer,
  sqn: Buffer,
  amf: Buffer,
): AuthVectors {
  const rand = crypto.randomBytes(16);
  return generateVectorsWithRand(ki, op, rand, sqn, amf);
}

/**
 * Generate authentication vectors with a given RAND (deterministic, for testing).
 */
export function generateVectorsWithRand(
  ki: Buffer,
  op: Buffer,
  rand: Buffer,
  sqn: Buffer,
  amf: Buffer,
): AuthVectors {
  // Step 1: OPc
  const opc = computeOPc(ki, op);

  // Step 2: TEMP = AES_K(RAND XOR OPc)
  const temp = aesEncrypt(ki, xor(rand, opc));

  // --- f1: MAC-A ---
  // IN1 = SQN || AMF || SQN || AMF (16 bytes)
  const in1 = Buffer.alloc(16);
  sqn.copy(in1, 0);
  amf.copy(in1, 6);
  sqn.copy(in1, 8);
  amf.copy(in1, 14);

  // OUT1 = AES_K(rotate(IN1 XOR OPc, r1) XOR TEMP XOR c1) XOR OPc
  const f1Input = xor(xor(rotate(xor(in1, opc), R1), temp), C1);
  const out1 = xor(aesEncrypt(ki, f1Input), opc);
  const macA = out1.subarray(0, 8);

  // --- f2 (RES) + f5 (AK) ---
  // OUT2 = AES_K(rotate(TEMP XOR OPc, r2) XOR c2) XOR OPc
  const f2Input = xor(rotate(xor(temp, opc), R2), C2);
  const out2 = xor(aesEncrypt(ki, f2Input), opc);
  const xres = out2.subarray(8, 16); // RES = OUT2[8..15]
  const ak = out2.subarray(0, 6);    // AK  = OUT2[0..5]

  // --- f3 (CK) ---
  // OUT3 = AES_K(rotate(TEMP XOR OPc, r3) XOR c3) XOR OPc
  const f3Input = xor(rotate(xor(temp, opc), R3), C3);
  const out3 = xor(aesEncrypt(ki, f3Input), opc);
  const ck = out3.subarray(0, 16);

  // --- f4 (IK) ---
  // OUT4 = AES_K(rotate(TEMP XOR OPc, r4) XOR c4) XOR OPc
  const f4Input = xor(rotate(xor(temp, opc), R4), C4);
  const out4 = xor(aesEncrypt(ki, f4Input), opc);
  const ik = out4.subarray(0, 16);

  // AUTN = (SQN XOR AK) || AMF || MAC-A
  const autn = Buffer.alloc(16);
  xor(sqn, ak).copy(autn, 0);
  amf.copy(autn, 6);
  macA.copy(autn, 8);

  return {
    rand: Buffer.from(rand),
    autn: Buffer.from(autn),
    xres: Buffer.from(xres),
    ck: Buffer.from(ck),
    ik: Buffer.from(ik),
    ak: Buffer.from(ak),
  };
}

/**
 * f5* — Anonymity Key for Resync (3GPP TS 35.206 §4.1)
 *
 * Uses rotation constant R5* = 0 (NOT R5=96) and
 * XOR constant C5* with byte[15] = 0x10 (NOT C5 with 0x08).
 *
 * @param ki   Subscriber key (16 bytes)
 * @param rand Random challenge (16 bytes)
 * @param op   Operator variant (16 bytes)
 * @returns    6-byte AK* used to de-conceal SQN_MS from AUTS
 */
export function f5Star(ki: Buffer, rand: Buffer, op: Buffer): Buffer {
  const opc = computeOPc(ki, op);
  const temp = aesEncrypt(ki, xor(rand, opc));

  // OUT5* = AES_K(rotate(TEMP XOR OPc, R5*) XOR C5*) XOR OPc
  // R5* = 0, C5*[15] = 0x10
  const f5StarInput = xor(rotate(xor(temp, opc), R5_STAR), C5_STAR);
  const out5Star = xor(aesEncrypt(ki, f5StarInput), opc);

  // AK* = OUT5*[0..5]
  return Buffer.from(out5Star.subarray(0, 6));
}

/**
 * f1* — MAC-S for Resync (3GPP TS 35.206 §4.1)
 *
 * Uses rotation constant R1* = 64 (same as R1) and
 * XOR constant C1* with byte[15] = 0x80 (NOT C1 with 0x00).
 *
 * @param ki   Subscriber key (16 bytes)
 * @param rand Random challenge (16 bytes)
 * @param sqn  Sequence number from device (6 bytes)
 * @param amf  Authentication Management Field (2 bytes, typically 0x0000 for resync)
 * @param op   Operator variant (16 bytes)
 * @returns    8-byte MAC-S
 */
export function f1Star(ki: Buffer, rand: Buffer, sqn: Buffer, amf: Buffer, op: Buffer): Buffer {
  const opc = computeOPc(ki, op);
  const temp = aesEncrypt(ki, xor(rand, opc));

  // IN1 = SQN || AMF || SQN || AMF (16 bytes)
  const in1 = Buffer.alloc(16);
  sqn.copy(in1, 0);
  amf.copy(in1, 6);
  sqn.copy(in1, 8);
  amf.copy(in1, 14);

  // OUT1* = AES_K(rotate(IN1 XOR OPc, R1*) XOR TEMP XOR C1*) XOR OPc
  // R1* = 64, C1*[15] = 0x80
  const f1StarInput = xor(xor(rotate(xor(in1, opc), R1_STAR), temp), C1_STAR);
  const out1Star = xor(aesEncrypt(ki, f1StarInput), opc);

  // MAC-S = OUT1*[0..7]
  return Buffer.from(out1Star.subarray(0, 8));
}

export interface AuTsValidationResult {
  valid: boolean;
  sqnMs?: Buffer;
}

/**
 * Validate AUTS and extract SQN_MS (3GPP TS 35.206 §6.3.3)
 *
 * AUTS = Concealed-SQN || MAC-S (6 + 8 = 14 bytes)
 * where Concealed-SQN = SQN_MS XOR AK*
 *
 * @param ki   Subscriber key (16 bytes)
 * @param rand RAND from the failed challenge (16 bytes)
 * @param auts AUTS from the device (14 bytes)
 * @param op   Operator variant (16 bytes)
 * @returns    { valid: true, sqnMs } if MAC-S matches, else { valid: false }
 */
export function validateAuts(
  ki: Buffer,
  rand: Buffer,
  auts: Buffer,
  op: Buffer,
): AuTsValidationResult {
  if (auts.length !== 14) {
    return { valid: false };
  }

  // AUTS = Concealed-SQN(6) || MAC-S(8)
  const concealedSqn = auts.subarray(0, 6);
  const receivedMacS = auts.subarray(6, 14);

  // Compute AK* = f5*(K, RAND, OP)
  const akStar = f5Star(ki, rand, op);

  // SQN_MS = Concealed-SQN XOR AK*
  const sqnMs = xor(concealedSqn, akStar);

  // Compute expected MAC-S = f1*(K, RAND, SQN_MS, AMF=0x0000, OP)
  const amfZero = Buffer.alloc(2, 0);
  const expectedMacS = f1Star(ki, rand, sqnMs, amfZero, op);

  // Compare MAC-S (constant-time)
  let diff = 0;
  for (let i = 0; i < 8; i++) {
    diff |= receivedMacS[i]! ^ expectedMacS[i]!;
  }

  if (diff !== 0) {
    return { valid: false };
  }

  return { valid: true, sqnMs: Buffer.from(sqnMs) };
}

/**
 * Generate AUTS for testing purposes — simulates a device generating AUTS
 * when its SQN is out of sync with the network.
 *
 * @param ki     Subscriber key (16 bytes)
 * @param rand   RAND from the challenge (16 bytes)
 * @param sqnMs  Device's SQN (6 bytes)
 * @param op     Operator variant (16 bytes)
 * @returns      AUTS (14 bytes)
 */
export function generateAuts(ki: Buffer, rand: Buffer, sqnMs: Buffer, op: Buffer): Buffer {
  // AK* = f5*(K, RAND, OP)
  const akStar = f5Star(ki, rand, op);

  // Concealed-SQN = SQN_MS XOR AK*
  const concealedSqn = xor(sqnMs, akStar);

  // MAC-S = f1*(K, RAND, SQN_MS, AMF=0x0000, OP)
  const amfZero = Buffer.alloc(2, 0);
  const macS = f1Star(ki, rand, sqnMs, amfZero, op);

  // AUTS = Concealed-SQN || MAC-S
  return Buffer.concat([concealedSqn, macS]);
}
