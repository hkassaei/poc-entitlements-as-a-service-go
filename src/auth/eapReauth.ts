/**
 * EAP-AKA Fast Re-authentication Orchestrator — RFC 4187 Section 5.1
 *
 * Handles the re-authentication exchange:
 *   RT1: Device presents re-auth identity → server sends AKA-Reauthentication challenge
 *   RT2: Device responds with encrypted counter + MAC → server verifies → issues new re-auth identity
 */

import crypto from 'node:crypto';
import { logger } from '../config/logger.js';
import { EAP_AKA } from '../config/constants.js';
import {
  EAP_CODE,
  EAP_TYPE_AKA,
  AKA_SUBTYPE,
  AT,
  encodeEapPacket,
  encodeEapToBase64,
  decodeEapPacket,
  type EapPacket,
  type EapAttribute,
} from './eapCodec.js';
import { computeMac, verifyMac, deriveReauthKeys } from './keyDerivation.js';
import { encryptAttributes, decryptAttributes } from './eapEncryption.js';
import { getReauthState, deleteReauthState, storeReauthState, generateReauthId } from './reauthStore.js';
import { createReauthSession, getReauthSession, deleteReauthSession } from './reauthSession.js';
import { handleInitialRequest } from './eapAka.js';

export interface ReauthChallengeResult {
  statusCode: 401;
  eapRelay: string;
  sessionId: string;
}

export interface ReauthResult {
  statusCode: number;
  reauthId?: string;
  eapRelay?: string;
  subscriberId?: string;
  sessionId?: string; // set when counter-too-small triggers full-auth fallback
}

let reauthIdentifierCounter = 128; // start at different offset from full auth to avoid collisions
function nextReauthIdentifier(): number {
  reauthIdentifierCounter = (reauthIdentifierCounter + 1) % 256;
  return reauthIdentifierCounter;
}

/**
 * Handle re-auth request (RT1): device presents a re-auth identity.
 *
 * 1. Look up re-auth state in Redis
 * 2. Check counter < MAX
 * 3. Build EAP-Request/AKA-Reauthentication with encrypted inner attributes
 * 4. Store re-auth session (90s TTL)
 * 5. Return 401 + challenge
 *
 * Returns null if re-auth state not found or counter exhausted (caller should fall back).
 */
export async function handleReauthRequest(
  reauthId: string,
  _clientIp: string,
): Promise<ReauthChallengeResult | null> {
  const state = await getReauthState(reauthId);
  if (!state) {
    logger.info({ reauthId }, 'Re-auth state not found, falling back to full auth');
    return null;
  }

  if (state.counter >= EAP_AKA.MAX_REAUTH_COUNTER) {
    logger.info({ reauthId, counter: state.counter }, 'Re-auth counter exhausted');
    await deleteReauthState(reauthId);
    return null;
  }

  const kAut = Buffer.from(state.kAut, 'base64');
  const kEncr = Buffer.from(state.kEncr, 'base64');
  const nonceS = crypto.randomBytes(16);
  const nextReauthId = generateReauthId();
  const iv = crypto.randomBytes(16);
  const identifier = nextReauthIdentifier();

  // Build inner attributes: AT_COUNTER, AT_NONCE_S, AT_NEXT_REAUTH_ID
  const counterBuf = Buffer.alloc(2);
  counterBuf.writeUInt16BE(state.counter, 0);

  const innerAttributes: EapAttribute[] = [
    { type: AT.AT_COUNTER, value: counterBuf },
    { type: AT.AT_NONCE_S, value: nonceS },
    { type: AT.AT_NEXT_REAUTH_ID, value: Buffer.from(nextReauthId, 'utf-8') },
  ];

  // Encrypt inner attributes
  const ciphertext = encryptAttributes(kEncr, iv, innerAttributes);

  // Build EAP-Request/AKA-Reauthentication with zeroed MAC
  const zeroMac = Buffer.alloc(16);
  const reauthPacket: EapPacket = {
    code: EAP_CODE.REQUEST,
    identifier,
    type: EAP_TYPE_AKA,
    subtype: AKA_SUBTYPE.REAUTHENTICATION,
    attributes: [
      { type: AT.AT_IV, value: iv },
      { type: AT.AT_ENCR_DATA, value: ciphertext },
      { type: AT.AT_MAC, value: zeroMac },
    ],
  };

  // Encode, compute MAC, patch it in
  const packetBytes = encodeEapPacket(reauthPacket);
  const mac = computeMac(kAut, packetBytes);
  const macOffset = findAtMacOffset(packetBytes);
  mac.copy(packetBytes, macOffset + 4); // +4 to skip type(1)+length(1)+reserved(2)

  const eapRelay = packetBytes.toString('base64');

  // Store re-auth session
  const sessionId = await createReauthSession({
    reauthId,
    nextReauthId,
    nonceS: nonceS.toString('base64'),
    counter: state.counter,
    identifier,
    kAut: state.kAut,
    kEncr: state.kEncr,
    mk: state.mk,
    subscriberId: state.subscriberId,
    imsi: state.imsi,
  });

  logger.info({ reauthId, sessionId, counter: state.counter }, 'Re-auth challenge sent');

  return { statusCode: 401, eapRelay, sessionId };
}

/**
 * Handle re-auth response (RT2): device sends EAP-Response/AKA-Reauthentication.
 *
 * 1. Look up re-auth session
 * 2. Verify MAC (before decryption — MAC covers ciphertext)
 * 3. Decrypt inner attributes
 * 4. Handle AT_COUNTER_TOO_SMALL → fall back to full auth
 * 5. Verify AT_COUNTER matches expected
 * 6. Derive new MSK'/EMSK'
 * 7. Rotate re-auth state (delete old, store new with incremented counter)
 * 8. Return 200 + new re-auth identity
 */
export async function handleReauthResponse(
  eapRelayBase64: string,
  sessionId: string,
  clientIp: string,
): Promise<ReauthResult> {
  const session = await getReauthSession(sessionId);
  if (!session) {
    logger.warn({ sessionId }, 'Re-auth session not found or expired');
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({ code: EAP_CODE.FAILURE, identifier: 0 }),
    };
  }

  // Decode the EAP-Response
  const rawBytes = Buffer.from(eapRelayBase64, 'base64');
  let packet: EapPacket;
  try {
    packet = decodeEapPacket(rawBytes);
  } catch (err) {
    logger.error({ err, sessionId }, 'Failed to decode re-auth EAP packet');
    await deleteReauthSession(sessionId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({ code: EAP_CODE.FAILURE, identifier: session.identifier }),
    };
  }

  // Handle AUTH_REJECT
  if (packet.subtype === AKA_SUBTYPE.AUTH_REJECT) {
    logger.warn({ sessionId }, 'Client sent AUTH_REJECT during re-auth');
    await deleteReauthSession(sessionId);
    await deleteReauthState(session.reauthId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({ code: EAP_CODE.FAILURE, identifier: session.identifier }),
    };
  }

  // Expect AKA-Reauthentication response
  if (packet.subtype !== AKA_SUBTYPE.REAUTHENTICATION) {
    logger.warn({ sessionId, subtype: packet.subtype }, 'Unexpected subtype in re-auth response');
    await deleteReauthSession(sessionId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({ code: EAP_CODE.FAILURE, identifier: session.identifier }),
    };
  }

  // Extract AT_IV, AT_ENCR_DATA, AT_MAC
  const atIv = findAttribute(packet.attributes, AT.AT_IV);
  const atEncrData = findAttribute(packet.attributes, AT.AT_ENCR_DATA);
  const atMac = findAttribute(packet.attributes, AT.AT_MAC);

  if (!atIv || !atEncrData || !atMac) {
    logger.warn({ sessionId }, 'Missing required re-auth attributes');
    await deleteReauthSession(sessionId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({ code: EAP_CODE.FAILURE, identifier: session.identifier }),
    };
  }

  // Verify MAC BEFORE decryption (MAC covers ciphertext)
  const kAut = Buffer.from(session.kAut, 'base64');
  const macOffset = findAtMacOffset(rawBytes);
  if (macOffset < 0 || !verifyMac(kAut, rawBytes, macOffset + 4, atMac.value)) {
    logger.warn({ sessionId }, 'Re-auth AT_MAC verification failed');
    await deleteReauthSession(sessionId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({ code: EAP_CODE.FAILURE, identifier: session.identifier }),
    };
  }

  // Decrypt inner attributes
  const kEncr = Buffer.from(session.kEncr, 'base64');
  let innerAttrs: EapAttribute[];
  try {
    innerAttrs = decryptAttributes(kEncr, atIv.value, atEncrData.value);
  } catch (err) {
    logger.error({ err, sessionId }, 'Failed to decrypt re-auth inner attributes');
    await deleteReauthSession(sessionId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({ code: EAP_CODE.FAILURE, identifier: session.identifier }),
    };
  }

  // Check for AT_COUNTER_TOO_SMALL → fall back to full auth
  const counterTooSmall = findAttribute(innerAttrs, AT.AT_COUNTER_TOO_SMALL);
  if (counterTooSmall) {
    logger.info({ sessionId, reauthId: session.reauthId }, 'Client sent AT_COUNTER_TOO_SMALL, falling back to full auth');
    await deleteReauthSession(sessionId);
    await deleteReauthState(session.reauthId);

    // Initiate full auth
    const fullAuthResult = await handleInitialRequest(session.imsi, clientIp);
    return {
      statusCode: 401,
      eapRelay: fullAuthResult.eapRelay,
      sessionId: fullAuthResult.sessionId,
    };
  }

  // Extract and verify AT_COUNTER
  const atCounter = findAttribute(innerAttrs, AT.AT_COUNTER);
  if (!atCounter) {
    logger.warn({ sessionId }, 'Missing AT_COUNTER in re-auth response');
    await deleteReauthSession(sessionId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({ code: EAP_CODE.FAILURE, identifier: session.identifier }),
    };
  }

  const clientCounter = atCounter.value.readUInt16BE(0);
  if (clientCounter !== session.counter) {
    logger.warn({ sessionId, expected: session.counter, got: clientCounter }, 'Re-auth counter mismatch');
    await deleteReauthSession(sessionId);
    await deleteReauthState(session.reauthId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({ code: EAP_CODE.FAILURE, identifier: session.identifier }),
    };
  }

  // Derive new session keys
  const mk = Buffer.from(session.mk, 'base64');
  const nonceS = Buffer.from(session.nonceS, 'base64');
  deriveReauthKeys(session.nextReauthId, session.counter, nonceS, mk);

  // Rotate re-auth state: delete old, store new
  await deleteReauthState(session.reauthId);
  await storeReauthState({
    subscriberId: session.subscriberId,
    imsi: session.imsi,
    mk: session.mk,
    kAut: session.kAut,
    kEncr: session.kEncr,
    counter: session.counter + 1,
    identity: session.nextReauthId,
  });

  // Clean up session
  await deleteReauthSession(sessionId);

  logger.info({
    reauthId: session.reauthId,
    nextReauthId: session.nextReauthId,
    counter: session.counter,
    subscriberId: session.subscriberId,
  }, 'Re-auth successful');

  return {
    statusCode: 200,
    reauthId: session.nextReauthId,
    subscriberId: session.subscriberId,
    eapRelay: encodeEapToBase64({
      code: EAP_CODE.SUCCESS,
      identifier: session.identifier,
    }),
  };
}

/**
 * Find a specific attribute in the attribute list.
 */
function findAttribute(
  attributes: EapAttribute[] | undefined,
  type: number,
): EapAttribute | undefined {
  return attributes?.find((a) => a.type === type);
}

/**
 * Find the byte offset of the AT_MAC attribute in raw packet bytes.
 */
function findAtMacOffset(buf: Buffer): number {
  let offset = 8; // skip EAP header + type/subtype/reserved
  while (offset + 2 <= buf.length) {
    const attrType = buf.readUInt8(offset);
    const attrLenWords = buf.readUInt8(offset + 1);
    const attrLen = attrLenWords * 4;

    if (attrType === AT.AT_MAC) {
      return offset;
    }

    offset += attrLen;
  }
  return -1;
}
