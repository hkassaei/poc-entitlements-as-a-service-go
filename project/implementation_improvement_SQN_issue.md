
# Summary
Lack of Sequence Number (SQN) Synchronization: In eapAka.ts, the SYNC_FAILURE case (where the SIM's SQN is out of sync with the
HSS) is currently a stub that simply fails the session. Do I interpret this correctly? In production, this must forward the AT_AUTS attribute to the HSS to resynchronize the counter and generate fresh vectors.

# CC's investigation result
● Let me verify your interpretation by reading the current handling and the relevant RFC context.

● Read 1 file (ctrl+o to expand)

● Now let me check what the mock-HSS supports — does it have a resync endpoint?

● Explore(Explore mock-HSS resync support)
  ⎿  Done (19 tool uses · 39.6k tokens · 35s)

● Yes, your interpretation is exactly right. Here's the full picture:

  Current behavior (eapAka.ts:190-201): When the device sends AKA_SUBTYPE.SYNC_FAILURE with AT_AUTS, the server logs a warning, kills the session, and returns EAP-Failure. The AUTS value is completely ignored.

  What should happen (RFC 4187 §5.1.4 + TS 33.102 §6.3.5):
  1. Extract AT_AUTS (14 bytes) from the device's response
  2. Forward AUTS + the original RAND to the HSS
  3. The HSS uses f5* (the "resync" variant of f5) to recover the device's actual SQN from AUTS
  4. HSS updates its stored SQN, generates fresh vectors with the corrected sequence number
  5. Server sends a new AKA-Challenge with the fresh vectors — same session, no EAP-Failure

  What's missing on both sides:
  - Mock-HSS: No /resync endpoint, no f5* implementation in milenage.ts
  - ECS: No AUTS extraction, no resync call, no re-challenge logic

  This matters in production because SQN drift is common — a device might authenticate against a different network element, or the HSS could restore from backup. Without resync, those devices get permanently
  locked out until someone manually fixes the SQN in the HSS database.

  Want me to plan and implement the full fix (mock-HSS f5* + resync endpoint, ECS resync flow)?

# SQN Resynchronization Implementation Plan

 Implement proper SYNC_FAILURE handling per RFC 4187 Section 6.2 and 3GPP TS 35.206.

## Summary

 When a device's SQN (Sequence Number) is out of sync with the network, it sends an EAP-Response/AKA-Synchronization-Failure containing AT_AUTS. The server must forward AT_AUTS to the HSS, which validates it
 using MILENAGE f5* and f1*, extracts the device's SQN_MS, updates its counter, and returns fresh authentication vectors. The server then issues a new AKA-Challenge.

 Current State

 The SYNC_FAILURE handler in src/auth/eapAka.ts:190-201 is a stub:
 if (packet.subtype === AKA_SUBTYPE.SYNC_FAILURE) {
   logger.warn({ sessionId, imsi: session.imsi }, 'Client sent SYNC_FAILURE');
   await deleteSession(sessionId);
   return { statusCode: 401, eapRelay: EAP-Failure };
 }

 ---
## Phase 1: MILENAGE Resync Functions (mock-hss/src/milenage.ts)

 f5* (Anonymity Key for Resync)

 Per 3GPP TS 35.206 Section 4.1:
 - Uses rotation constant R5* = 0 (NOT R5=96)
 - Uses XOR constant C5* with byte[15] = 0x10 (NOT C5 with 0x08)
 - Output: 6-byte AK* used to de-conceal SQN_MS from AUTS

 export function f5Star(k: Buffer, rand: Buffer, op: Buffer): Buffer

 f1* (MAC-S for Resync)

 Per 3GPP TS 35.206 Section 4.1:
 - Uses rotation constant R1* = 64 (same as R1)
 - Uses XOR constant C1* with byte[15] = 0x80 (NOT C1 with 0x00)
 - Input: K, RAND, SQN_MS, AMF (set to 0x0000 for resync)
 - Output: 8-byte MAC-S

 export function f1Star(k: Buffer, rand: Buffer, sqn: Buffer, amf: Buffer, op: Buffer): Buffer

 validateAuts

 Validates the AUTS and extracts SQN_MS:

 export function validateAuts(
   k: Buffer, rand: Buffer, auts: Buffer, op: Buffer
 ): { valid: boolean; sqnMs?: Buffer }

 Logic:
 1. AUTS = Concealed-SQN || MAC-S (6 + 8 = 14 bytes)
 2. Compute AK* = f5*(K, RAND, OP)
 3. SQN_MS = Concealed-SQN XOR AK*
 4. Compute expected MAC-S = f1*(K, RAND, SQN_MS, AMF=0x0000, OP)
 5. Compare MAC-S; return { valid, sqnMs }

 ---
## Phase 2: Mock-HSS /resync Endpoint (mock-hss/src/index.ts)

 POST /resync

 interface ResyncRequest {
   imsi: string;
   rand: string;  // base64, RAND from failed challenge
   auts: string;  // base64, 14 bytes from AT_AUTS
 }

 interface ResyncResponse {
   success: true;
   rand: string;   // base64, new RAND
   autn: string;   // base64, new AUTN
   xres: string;   // base64
   ck: string;     // base64
   ik: string;     // base64
 }

 Flow:
 1. Look up subscriber by IMSI, decrypt Ki
 2. Call validateAuts(ki, rand, auts, op)
 3. If invalid: return 400 { success: false, error: "AUTS validation failed" }
 4. Update subscriber.sqn to SQN_MS + delta (e.g., +32 for margin)
 5. Generate new vectors with updated SQN
 6. Return fresh vectors

 Update GET /vectors

 Add SQN increment after vector generation to prevent replay attacks.

 ---
## Phase 3: ECS HSS Client (src/auth/eapAkaVectors.ts)

 Add resyncVectors function

 export interface ResyncResult {
   success: boolean;
   vectors?: AkaVectors;
   error?: string;
 }

 export async function resyncVectors(
   imsi: string, rand: Buffer, auts: Buffer
 ): Promise<ResyncResult>

 Calls POST ${config.hssUrl}/resync and returns parsed vectors or error.

 ---
## Phase 4: EAP Session Update (src/auth/eapSession.ts)

 Add updateSession function

 export async function updateSession(
   sessionId: string,
   updates: Partial<Pick<EapSessionData, 'xres' | 'ck' | 'ik' | 'kAut' | 'kEncr' | 'mk' | 'rand' | 'identifier'>>
 ): Promise<void>

 Add rand to EapSessionData

 Store the RAND used in the challenge so it can be forwarded during resync.

 ---
## Phase 5: SYNC_FAILURE Handler (src/auth/eapAka.ts)

 Replace stub with full implementation:

 1. Extract AT_AUTS (14 bytes) from SYNC_FAILURE packet
 2. If missing/invalid AT_AUTS: delete session, return EAP-Failure
 3. Get original RAND from session
 4. Call resyncVectors(imsi, rand, auts)
 5. If resync fails: delete session, return EAP-Failure
 6. Derive new keys from fresh vectors
 7. Update session with new vectors, keys, and incremented identifier
 8. Build new AKA-Challenge with fresh RAND/AUTN
 9. Return 401 + new challenge (same sessionId)

 Add AT_AUTS to eapCodec.ts

 AT_AUTS: 3,  // RFC 4187 Section 10.9
 // Decode: 14-byte value (type=1, length=4, value=14 bytes)

 ---
## Phase 6: Tests

 Unit Tests (mock-hss/tests/milenage.test.ts)

 - f5* produces 6-byte AK* different from f5's AK
 - f1* produces 8-byte MAC-S different from f1's MAC
 - validateAuts returns valid=true for correct AUTS
 - validateAuts returns valid=false for tampered AUTS
 - Round-trip: generate AUTS → validateAuts extracts same SQN_MS

 Integration Tests (tests/integration/eapAka.integration.test.ts)

 1. SYNC_FAILURE → new challenge → successful auth
 2. SYNC_FAILURE with invalid AT_AUTS → EAP-Failure
 3. SYNC_FAILURE without AT_AUTS → EAP-Failure

 ---
 Files Summary

 mock-hss (3 files):
 - mock-hss/src/milenage.ts — Add f5*, f1*, validateAuts
 - mock-hss/src/index.ts — Add POST /resync, update /vectors SQN handling
 - mock-hss/tests/milenage.test.ts — Add resync function tests

 ECS (5 files):
 - src/auth/eapCodec.ts — Add AT_AUTS decode
 - src/auth/eapAkaVectors.ts — Add resyncVectors()
 - src/auth/eapSession.ts — Add updateSession(), add rand to EapSessionData
 - src/auth/eapAka.ts — Replace SYNC_FAILURE stub, store rand in session
 - tests/integration/eapAka.integration.test.ts — Add resync integration tests

 ---
 Verification

 1. cd mock-hss && npm test — milenage unit tests pass
 2. npm run build — TypeScript compiles
 3. npm run test — unit tests pass
 4. docker compose up -d && npm run test:integration — all integration tests pass