/**
 * EAP-AKA Orchestrator
 *
 * Central state machine for the two-round-trip EAP-AKA handshake:
 *
 * RT1: Device sends IMSI → ECS challenges with EAP-Request/AKA-Challenge
 * RT2: Device sends EAP-Response with AT_RES + AT_MAC → ECS verifies → issues token
 */

import crypto from 'node:crypto';
import { logger } from '../config/logger.js';
import { EAP_STATE, TOKEN_TYPES } from '../config/constants.js';
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
import { buildIdentity, deriveKeys, computeMac, verifyMac } from './keyDerivation.js';
import { fetchVectors, HssSubscriberNotFoundError } from './eapAkaVectors.js';
import { createSession, getSession, deleteSession } from './eapSession.js';
import { generateToken, findSubscriberByImsi } from './tokenService.js';

export interface ChallengeResult {
  statusCode: number;
  eapRelay: string;
  sessionId: string;
}

export interface AuthResult {
  statusCode: number;
  token?: string;
  eapRelay?: string; // EAP-Success or EAP-Failure base64
  subscriberId?: string;
}

let identifierCounter = 0;
function nextIdentifier(): number {
  identifierCounter = (identifierCounter + 1) % 256;
  return identifierCounter;
}

/**
 * Handle the initial EAP-AKA request (Round Trip 1).
 *
 * 1. Fetch auth vectors from HSS
 * 2. Derive keys (MK → K_encr, K_aut, MSK, EMSK)
 * 3. Build EAP-Request/AKA-Challenge with AT_RAND, AT_AUTN, AT_MAC
 * 4. Store session in Redis
 * 5. Return 401 + eap_relay + session ID
 */
export async function handleInitialRequest(
  imsi: string,
  clientIp: string,
): Promise<ChallengeResult> {
  // Fetch vectors from mock HSS
  const vectors = await fetchVectors(imsi);

  // Derive keys
  const identity = buildIdentity(imsi);
  const keys = deriveKeys(identity, vectors.ik, vectors.ck);

  const identifier = nextIdentifier();

  // Build EAP-Request/AKA-Challenge packet with zeroed MAC first
  const zeroMac = Buffer.alloc(16);
  const challengePacket: EapPacket = {
    code: EAP_CODE.REQUEST,
    identifier,
    type: EAP_TYPE_AKA,
    subtype: AKA_SUBTYPE.CHALLENGE,
    attributes: [
      { type: AT.AT_RAND, value: vectors.rand },
      { type: AT.AT_AUTN, value: vectors.autn },
      { type: AT.AT_MAC, value: zeroMac },
    ],
  };

  // Encode with zeroed MAC, compute real MAC, then patch it in
  const packetBytes = encodeEapPacket(challengePacket);
  const mac = computeMac(keys.kAut, packetBytes);

  // Find the MAC offset: header(8) + AT_RAND(20) + AT_AUTN(20) + AT_MAC header(4) = offset 52
  // AT_MAC attribute: type(1) + length(1) + reserved(2) + mac(16)
  // The MAC value starts at byte 52+4 = 56... Let's find it properly
  const macAttrOffset = findAtMacOffset(packetBytes);
  mac.copy(packetBytes, macAttrOffset + 4); // +4 to skip type(1)+length(1)+reserved(2)

  const eapRelay = packetBytes.toString('base64');

  // Store session
  const sessionId = await createSession({
    imsi,
    rand: vectors.rand,
    xres: vectors.xres,
    ck: vectors.ck,
    ik: vectors.ik,
    identifier,
    kAut: keys.kAut,
    kEncr: keys.kEncr,
  });

  logger.info({ imsi, sessionId }, 'EAP-AKA challenge sent');

  return {
    statusCode: 401,
    eapRelay,
    sessionId,
  };
}

/**
 * Handle an EAP-AKA response (Round Trip 2).
 *
 * 1. Look up session
 * 2. Decode EAP-Response packet
 * 3. Verify AT_RES matches stored XRES (timing-safe)
 * 4. Verify AT_MAC with K_aut
 * 5. Generate auth token
 * 6. Return 200 + token
 */
export async function handleEapResponse(
  eapRelayBase64: string,
  sessionId: string,
  clientIp: string,
): Promise<AuthResult> {
  // Retrieve session
  const session = await getSession(sessionId);
  if (!session) {
    logger.warn({ sessionId }, 'EAP session not found or expired');
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({
        code: EAP_CODE.FAILURE,
        identifier: 0,
      }),
    };
  }

  if (session.state !== EAP_STATE.CHALLENGE_SENT) {
    logger.warn({ sessionId, state: session.state }, 'EAP session in unexpected state');
    await deleteSession(sessionId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({
        code: EAP_CODE.FAILURE,
        identifier: parseInt(session.identifier, 10),
      }),
    };
  }

  // Decode the EAP-Response
  const rawBytes = Buffer.from(eapRelayBase64, 'base64');
  let packet: EapPacket;
  try {
    packet = decodeEapPacket(rawBytes);
  } catch (err) {
    logger.error({ err, sessionId }, 'Failed to decode EAP packet');
    await deleteSession(sessionId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({
        code: EAP_CODE.FAILURE,
        identifier: parseInt(session.identifier, 10),
      }),
    };
  }

  // Handle AUTH_REJECT
  if (packet.subtype === AKA_SUBTYPE.AUTH_REJECT) {
    logger.warn({ sessionId, imsi: session.imsi }, 'Client sent AUTH_REJECT');
    await deleteSession(sessionId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({
        code: EAP_CODE.FAILURE,
        identifier: parseInt(session.identifier, 10),
      }),
    };
  }

  // Handle SYNC_FAILURE (AT_AUTS)
  if (packet.subtype === AKA_SUBTYPE.SYNC_FAILURE) {
    logger.warn({ sessionId, imsi: session.imsi }, 'Client sent SYNC_FAILURE');
    await deleteSession(sessionId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({
        code: EAP_CODE.FAILURE,
        identifier: parseInt(session.identifier, 10),
      }),
    };
  }

  // Expect AKA-Challenge response
  if (packet.subtype !== AKA_SUBTYPE.CHALLENGE) {
    logger.warn({ sessionId, subtype: packet.subtype }, 'Unexpected EAP subtype');
    await deleteSession(sessionId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({
        code: EAP_CODE.FAILURE,
        identifier: parseInt(session.identifier, 10),
      }),
    };
  }

  // Extract AT_RES and AT_MAC from the response
  const atRes = findAttribute(packet.attributes, AT.AT_RES);
  const atMac = findAttribute(packet.attributes, AT.AT_MAC);

  if (!atRes || !atMac) {
    logger.warn({ sessionId, hasRes: !!atRes, hasMac: !!atMac }, 'Missing required attributes');
    await deleteSession(sessionId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({
        code: EAP_CODE.FAILURE,
        identifier: parseInt(session.identifier, 10),
      }),
    };
  }

  // Verify AT_RES == XRES (timing-safe)
  const expectedXres = Buffer.from(session.xres, 'base64');
  if (
    atRes.value.length !== expectedXres.length ||
    !crypto.timingSafeEqual(atRes.value, expectedXres)
  ) {
    logger.warn({ sessionId, imsi: session.imsi }, 'AT_RES mismatch');
    await deleteSession(sessionId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({
        code: EAP_CODE.FAILURE,
        identifier: parseInt(session.identifier, 10),
      }),
    };
  }

  // Verify AT_MAC
  const kAut = Buffer.from(session.kAut, 'base64');
  const macOffset = findAtMacOffset(rawBytes);
  if (macOffset < 0 || !verifyMac(kAut, rawBytes, macOffset + 4, atMac.value)) {
    logger.warn({ sessionId, imsi: session.imsi }, 'AT_MAC verification failed');
    await deleteSession(sessionId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({
        code: EAP_CODE.FAILURE,
        identifier: parseInt(session.identifier, 10),
      }),
    };
  }

  // Auth successful — find or create subscriber reference, generate token
  const subscriberId = await findSubscriberByImsi(session.imsi);
  if (!subscriberId) {
    logger.error({ imsi: session.imsi }, 'Subscriber not found in DB after successful auth');
    await deleteSession(sessionId);
    return {
      statusCode: 401,
      eapRelay: encodeEapToBase64({
        code: EAP_CODE.FAILURE,
        identifier: parseInt(session.identifier, 10),
      }),
    };
  }

  const tokenInfo = await generateToken(subscriberId, TOKEN_TYPES.AUTH, clientIp);

  // Clean up session
  await deleteSession(sessionId);

  const successIdentifier = parseInt(session.identifier, 10);

  logger.info({ imsi: session.imsi, sessionId, subscriberId }, 'EAP-AKA authentication successful');

  return {
    statusCode: 200,
    token: tokenInfo.tokenValue,
    subscriberId,
    eapRelay: encodeEapToBase64({
      code: EAP_CODE.SUCCESS,
      identifier: successIdentifier,
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
 * Scans attributes starting at offset 8.
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
