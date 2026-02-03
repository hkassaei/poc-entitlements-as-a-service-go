import { describe, it, expect } from 'vitest';
import crypto from 'node:crypto';
import {
  encryptWithDek,
  decryptWithDek,
  wrapDek,
  unwrapDek,
  zeroBuffer,
  LocalKeyManager,
} from '../src/kms.js';

describe('KMS — AES-256-GCM encrypt/decrypt', () => {
  it('round-trips plaintext through encrypt/decrypt', () => {
    const dek = crypto.randomBytes(32);
    const plaintext = Buffer.from('Hello, MILENAGE!');

    const encrypted = encryptWithDek(dek, plaintext);
    const decrypted = decryptWithDek(dek, encrypted);

    expect(decrypted.toString()).toBe('Hello, MILENAGE!');
  });

  it('encrypted output is IV(12) + ciphertext + authTag(16)', () => {
    const dek = crypto.randomBytes(32);
    const plaintext = Buffer.from('test');

    const encrypted = encryptWithDek(dek, plaintext);
    // 12 (IV) + 4 (ciphertext same length as plaintext) + 16 (tag) = 32
    expect(encrypted.length).toBe(32);
  });

  it('fails to decrypt with wrong key', () => {
    const dek1 = crypto.randomBytes(32);
    const dek2 = crypto.randomBytes(32);
    const plaintext = Buffer.from('secret');

    const encrypted = encryptWithDek(dek1, plaintext);
    expect(() => decryptWithDek(dek2, encrypted)).toThrow();
  });
});

describe('KMS — DEK wrap/unwrap', () => {
  it('round-trips DEK through wrap/unwrap', () => {
    const kek = crypto.randomBytes(32);
    const dek = crypto.randomBytes(32);

    const wrapped = wrapDek(kek, dek);
    const unwrapped = unwrapDek(kek, wrapped);

    expect(unwrapped.toString('hex')).toBe(dek.toString('hex'));
  });

  it('wrapped DEK differs from original', () => {
    const kek = crypto.randomBytes(32);
    const dek = crypto.randomBytes(32);

    const wrapped = wrapDek(kek, dek);
    expect(wrapped.toString('hex')).not.toBe(dek.toString('hex'));
  });

  it('rejects non-32-byte inputs', () => {
    const kek = crypto.randomBytes(16);
    const dek = crypto.randomBytes(32);
    expect(() => wrapDek(kek, dek)).toThrow('KEK and DEK must both be 32 bytes');
  });
});

describe('KMS — zeroBuffer', () => {
  it('fills buffer with zeros', () => {
    const buf = Buffer.from('sensitive data here');
    zeroBuffer(buf);
    expect(buf.every((b) => b === 0)).toBe(true);
  });
});

describe('LocalKeyManager', () => {
  it('unwraps a DEK correctly', () => {
    const kekHex = '000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f';
    const kek = Buffer.from(kekHex, 'hex');
    const dek = crypto.randomBytes(32);
    const wrapped = wrapDek(kek, dek);

    const manager = new LocalKeyManager(kekHex);
    const unwrapped = manager.unwrapDek(wrapped);
    expect(unwrapped.toString('hex')).toBe(dek.toString('hex'));
  });

  it('rejects invalid KEK length', () => {
    expect(() => new LocalKeyManager('0011')).toThrow('LOCAL_KEK_HEX must be 64 hex chars');
  });
});
