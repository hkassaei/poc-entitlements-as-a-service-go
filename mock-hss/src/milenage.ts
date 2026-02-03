import crypto from 'node:crypto';

// MILENAGE constants (3GPP TS 35.206 §4.1)
const C1 = Buffer.alloc(16, 0); // 0x00...00
const C2 = Buffer.alloc(16, 0); C2[15] = 0x01;
const C3 = Buffer.alloc(16, 0); C3[15] = 0x02;
const C4 = Buffer.alloc(16, 0); C4[15] = 0x04;
const C5 = Buffer.alloc(16, 0); C5[15] = 0x08;

const R1 = 64;
const R2 = 0;
const R3 = 32;
const R4 = 64;
const R5 = 96;

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
