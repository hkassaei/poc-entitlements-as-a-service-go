/**
 * EAP-AKA Attribute Encryption — RFC 4187 Section 10.12
 *
 * AES-128-CBC encrypt/decrypt for AT_ENCR_DATA.
 * Inner attributes are serialized, padded with AT_PADDING to 16-byte boundary,
 * then encrypted. We use setAutoPadding(false) because AT_PADDING handles alignment.
 */

import crypto from 'node:crypto';
import { AT, encodeAttribute, type EapAttribute } from './eapCodec.js';

/**
 * Encrypt a list of inner attributes into AT_ENCR_DATA ciphertext.
 *
 * 1. Serialize inner attributes via encodeAttribute()
 * 2. Append AT_PADDING if total isn't 16-byte aligned
 * 3. AES-128-CBC encrypt with no auto-padding
 */
export function encryptAttributes(
  kEncr: Buffer,
  iv: Buffer,
  innerAttributes: EapAttribute[],
): Buffer {
  // Serialize inner attributes
  const attrBuffers = innerAttributes.map((attr) => encodeAttribute(attr));
  let plaintext = Buffer.concat(attrBuffers);

  // Pad to 16-byte boundary with AT_PADDING
  const remainder = plaintext.length % 16;
  if (remainder !== 0) {
    const paddingNeeded = 16 - remainder;
    // AT_PADDING: type(1) + length_in_4_byte_words(1) + zero_bytes
    // Minimum AT_PADDING is 4 bytes (length=1). Must be multiple of 4.
    // paddingNeeded is guaranteed to be a multiple of 4 since inner attributes are 4-byte aligned
    const paddingAttr = encodeAttribute({
      type: AT.AT_PADDING,
      value: Buffer.alloc(paddingNeeded - 2), // subtract type+length bytes
    });
    plaintext = Buffer.concat([plaintext, paddingAttr]);
  }

  const cipher = crypto.createCipheriv('aes-128-cbc', kEncr, iv);
  cipher.setAutoPadding(false);
  return Buffer.concat([cipher.update(plaintext), cipher.final()]);
}

/**
 * Decrypt AT_ENCR_DATA ciphertext and parse inner attributes.
 * Strips AT_PADDING from the result.
 */
export function decryptAttributes(
  kEncr: Buffer,
  iv: Buffer,
  ciphertext: Buffer,
): EapAttribute[] {
  const decipher = crypto.createDecipheriv('aes-128-cbc', kEncr, iv);
  decipher.setAutoPadding(false);
  const plaintext = Buffer.concat([decipher.update(ciphertext), decipher.final()]);

  // Parse inner attributes
  const attributes: EapAttribute[] = [];
  let offset = 0;
  while (offset < plaintext.length) {
    if (offset + 2 > plaintext.length) break;

    const type = plaintext.readUInt8(offset);
    const lengthWords = plaintext.readUInt8(offset + 1);
    const totalBytes = lengthWords * 4;

    if (totalBytes === 0 || offset + totalBytes > plaintext.length) break;

    // Skip AT_PADDING
    if (type === AT.AT_PADDING) {
      offset += totalBytes;
      continue;
    }

    // Re-use the codec's decode logic by extracting the attribute slice
    // and decoding manually based on type
    let value: Buffer;
    switch (type) {
      case AT.AT_COUNTER: {
        value = Buffer.alloc(2);
        plaintext.copy(value, 0, offset + 2, offset + 4);
        break;
      }
      case AT.AT_COUNTER_TOO_SMALL: {
        value = Buffer.alloc(0);
        break;
      }
      case AT.AT_NONCE_S: {
        value = Buffer.alloc(16);
        plaintext.copy(value, 0, offset + 4, offset + 20);
        break;
      }
      case AT.AT_NEXT_REAUTH_ID: {
        const actualLen = plaintext.readUInt16BE(offset + 2);
        value = Buffer.alloc(actualLen);
        plaintext.copy(value, 0, offset + 4, offset + 4 + actualLen);
        break;
      }
      default: {
        value = Buffer.alloc(totalBytes - 2);
        plaintext.copy(value, 0, offset + 2, offset + totalBytes);
        break;
      }
    }

    attributes.push({ type, value });
    offset += totalBytes;
  }

  return attributes;
}
