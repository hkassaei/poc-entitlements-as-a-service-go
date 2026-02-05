/**
 * EAP-AKA Packet Codec — RFC 4187
 *
 * Binary encode/decode for EAP packets carrying AKA attributes.
 *
 * EAP header (4 bytes): Code(1) | Identifier(1) | Length(2)
 * For type-specific packets, 4 more bytes: Type(1) | Subtype(1) | Reserved(2)
 * Then TLV attributes: Type(1) | LengthIn4ByteWords(1) | Value(padded)
 */

export const EAP_CODE = {
  REQUEST: 1,
  RESPONSE: 2,
  SUCCESS: 3,
  FAILURE: 4,
} as const;

export const EAP_TYPE_AKA = 23;

export const AKA_SUBTYPE = {
  CHALLENGE: 1,
  AUTH_REJECT: 2,
  SYNC_FAILURE: 4,
  IDENTITY: 5,
  REAUTHENTICATION: 13,
} as const;

export const AT = {
  AT_RAND: 1,
  AT_AUTN: 2,
  AT_RES: 3,
  AT_AUTS: 4,
  AT_PADDING: 6,
  AT_MAC: 11,
  AT_NEXT_REAUTH_ID: 14,
  AT_COUNTER: 19,
  AT_COUNTER_TOO_SMALL: 20,
  AT_NONCE_S: 21,
  AT_IV: 129,
  AT_ENCR_DATA: 130,
} as const;

export type EapCode = (typeof EAP_CODE)[keyof typeof EAP_CODE];
export type AkaSubtype = (typeof AKA_SUBTYPE)[keyof typeof AKA_SUBTYPE];

export interface EapAttribute {
  type: number;
  value: Buffer;
}

export interface EapPacket {
  code: EapCode;
  identifier: number;
  type?: number;
  subtype?: AkaSubtype;
  attributes?: EapAttribute[];
}

/**
 * Encode an EAP attribute into its TLV binary representation.
 */
export function encodeAttribute(attr: EapAttribute): Buffer {
  const type = attr.type;
  let valuePayload: Buffer;

  switch (type) {
    case AT.AT_RAND:
    case AT.AT_AUTN:
    case AT.AT_MAC:
    case AT.AT_IV:
    case AT.AT_NONCE_S: {
      // 2 reserved bytes + 16 bytes value = 18 bytes payload, length=5 (20 bytes total)
      valuePayload = Buffer.alloc(18);
      attr.value.copy(valuePayload, 2, 0, 16);
      break;
    }
    case AT.AT_RES: {
      // 2-byte bit-length prefix + value, total attribute must be multiple of 4 bytes
      // Total = type(1) + length(1) + bitlen(2) + value(padded)
      const bitLength = attr.value.length * 8;
      const totalAttrLen = Math.ceil((2 + 2 + attr.value.length) / 4) * 4;
      valuePayload = Buffer.alloc(totalAttrLen - 2); // subtract type+length bytes
      valuePayload.writeUInt16BE(bitLength, 0);
      attr.value.copy(valuePayload, 2);
      break;
    }
    case AT.AT_AUTS: {
      // 2 reserved bytes + 14 bytes AUTS = 16 bytes payload, length=4
      valuePayload = Buffer.alloc(16);
      attr.value.copy(valuePayload, 2, 0, 14);
      break;
    }
    case AT.AT_COUNTER: {
      // 2-byte big-endian counter value, length=1 (4 bytes total)
      valuePayload = Buffer.alloc(2);
      valuePayload.writeUInt16BE(attr.value.readUInt16BE(0), 0);
      break;
    }
    case AT.AT_COUNTER_TOO_SMALL: {
      // 2 reserved bytes, no value, length=1 (4 bytes total)
      valuePayload = Buffer.alloc(2);
      break;
    }
    case AT.AT_PADDING: {
      // attr.value.length contains the padding bytes (zeros)
      valuePayload = Buffer.alloc(attr.value.length);
      break;
    }
    case AT.AT_ENCR_DATA: {
      // 2 reserved bytes + variable-length ciphertext
      valuePayload = Buffer.alloc(2 + attr.value.length);
      attr.value.copy(valuePayload, 2);
      break;
    }
    case AT.AT_NEXT_REAUTH_ID: {
      // 2-byte actual-length prefix + UTF-8 identity + padding to 4-byte boundary
      const identityBytes = attr.value;
      const actualLen = identityBytes.length;
      const totalAttrLen = Math.ceil((2 + 2 + actualLen) / 4) * 4;
      valuePayload = Buffer.alloc(totalAttrLen - 2); // subtract type+length bytes
      valuePayload.writeUInt16BE(actualLen, 0);
      identityBytes.copy(valuePayload, 2);
      break;
    }
    default: {
      // Generic: pad value to 4-byte boundary (minus 2 for type+length header)
      const totalPayload = Math.ceil(attr.value.length / 4) * 4;
      valuePayload = Buffer.alloc(totalPayload);
      attr.value.copy(valuePayload, 0);
      break;
    }
  }

  const totalLength = 2 + valuePayload.length; // type(1) + length(1) + payload
  const buf = Buffer.alloc(totalLength);
  buf.writeUInt8(type, 0);
  buf.writeUInt8(totalLength / 4, 1); // length in 4-byte words
  valuePayload.copy(buf, 2);
  return buf;
}

/**
 * Decode a single EAP attribute from a buffer at the given offset.
 * Returns the parsed attribute and the number of bytes consumed.
 */
function decodeAttribute(buf: Buffer, offset: number): { attr: EapAttribute; bytesRead: number } {
  if (offset + 2 > buf.length) {
    throw new Error('Truncated attribute header');
  }

  const type = buf.readUInt8(offset);
  const lengthWords = buf.readUInt8(offset + 1);
  const totalBytes = lengthWords * 4;

  if (offset + totalBytes > buf.length) {
    throw new Error(`Truncated attribute value: type=${type}, expected ${totalBytes} bytes`);
  }

  let value: Buffer;

  switch (type) {
    case AT.AT_RAND:
    case AT.AT_AUTN:
    case AT.AT_MAC:
    case AT.AT_IV:
    case AT.AT_NONCE_S: {
      // Skip 2 reserved bytes, read 16 bytes
      value = Buffer.alloc(16);
      buf.copy(value, 0, offset + 4, offset + 20);
      break;
    }
    case AT.AT_RES: {
      // 2-byte bit-length prefix after type+length header
      const bitLength = buf.readUInt16BE(offset + 2);
      const byteLength = bitLength / 8;
      value = Buffer.alloc(byteLength);
      buf.copy(value, 0, offset + 4, offset + 4 + byteLength);
      break;
    }
    case AT.AT_AUTS: {
      // Skip 2 reserved bytes, read 14 bytes
      value = Buffer.alloc(14);
      buf.copy(value, 0, offset + 4, offset + 18);
      break;
    }
    case AT.AT_COUNTER: {
      // 2-byte big-endian counter after type+length header
      value = Buffer.alloc(2);
      buf.copy(value, 0, offset + 2, offset + 4);
      break;
    }
    case AT.AT_COUNTER_TOO_SMALL: {
      // No meaningful value — 2 reserved bytes
      value = Buffer.alloc(0);
      break;
    }
    case AT.AT_PADDING: {
      // Padding bytes after type+length header
      value = Buffer.alloc(totalBytes - 2);
      buf.copy(value, 0, offset + 2, offset + totalBytes);
      break;
    }
    case AT.AT_ENCR_DATA: {
      // Skip 2 reserved bytes, rest is ciphertext
      const ciphertextLen = totalBytes - 4; // subtract type(1)+length(1)+reserved(2)
      value = Buffer.alloc(ciphertextLen);
      buf.copy(value, 0, offset + 4, offset + 4 + ciphertextLen);
      break;
    }
    case AT.AT_NEXT_REAUTH_ID: {
      // 2-byte actual-length prefix after type+length header
      const actualLen = buf.readUInt16BE(offset + 2);
      value = Buffer.alloc(actualLen);
      buf.copy(value, 0, offset + 4, offset + 4 + actualLen);
      break;
    }
    default: {
      // Generic: everything after the 2-byte header
      value = Buffer.alloc(totalBytes - 2);
      buf.copy(value, 0, offset + 2, offset + totalBytes);
      break;
    }
  }

  return { attr: { type, value }, bytesRead: totalBytes };
}

/**
 * Encode a full EAP packet to a Buffer.
 */
export function encodeEapPacket(packet: EapPacket): Buffer {
  // EAP-Success and EAP-Failure are just 4 bytes: Code | Id | Length(=4)
  if (packet.code === EAP_CODE.SUCCESS || packet.code === EAP_CODE.FAILURE) {
    const buf = Buffer.alloc(4);
    buf.writeUInt8(packet.code, 0);
    buf.writeUInt8(packet.identifier, 1);
    buf.writeUInt16BE(4, 2);
    return buf;
  }

  // Build attribute bytes
  const attrBuffers: Buffer[] = [];
  if (packet.attributes) {
    for (const attr of packet.attributes) {
      attrBuffers.push(encodeAttribute(attr));
    }
  }
  const attrBytes = Buffer.concat(attrBuffers);

  // Total: EAP header(4) + Type(1) + Subtype(1) + Reserved(2) + attributes
  const totalLength = 8 + attrBytes.length;
  const buf = Buffer.alloc(totalLength);

  buf.writeUInt8(packet.code, 0);
  buf.writeUInt8(packet.identifier, 1);
  buf.writeUInt16BE(totalLength, 2);
  buf.writeUInt8(packet.type ?? EAP_TYPE_AKA, 4);
  buf.writeUInt8(packet.subtype ?? 0, 5);
  // bytes 6-7 reserved (already zero)
  attrBytes.copy(buf, 8);

  return buf;
}

/**
 * Decode a Buffer into an EapPacket.
 */
export function decodeEapPacket(buf: Buffer): EapPacket {
  if (buf.length < 4) {
    throw new Error('EAP packet too short');
  }

  const code = buf.readUInt8(0) as EapCode;
  const identifier = buf.readUInt8(1);
  const length = buf.readUInt16BE(2);

  if (buf.length < length) {
    throw new Error(`EAP packet truncated: header says ${length}, got ${buf.length}`);
  }

  // Success/Failure: no type or attributes
  if (code === EAP_CODE.SUCCESS || code === EAP_CODE.FAILURE) {
    return { code, identifier };
  }

  if (length < 8) {
    throw new Error('EAP-AKA packet too short for type/subtype header');
  }

  const type = buf.readUInt8(4);
  const subtype = buf.readUInt8(5) as AkaSubtype;
  // bytes 6-7 reserved

  const attributes: EapAttribute[] = [];
  let offset = 8;
  while (offset < length) {
    const { attr, bytesRead } = decodeAttribute(buf, offset);
    attributes.push(attr);
    offset += bytesRead;
  }

  return { code, identifier, type, subtype, attributes };
}

/**
 * Encode an EAP packet and return as base64 string.
 */
export function encodeEapToBase64(packet: EapPacket): string {
  return encodeEapPacket(packet).toString('base64');
}

/**
 * Decode a base64-encoded EAP packet.
 */
export function decodeEapFromBase64(b64: string): EapPacket {
  return decodeEapPacket(Buffer.from(b64, 'base64'));
}
