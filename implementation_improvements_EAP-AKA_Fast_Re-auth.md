 EAP-AKA Fast Re-authentication Implementation Plan

 Replace opaque token-based fast-auth with RFC 4187 Section 5.1 cryptographic fast re-authentication.

 # Summary

 After full EAP-AKA auth, the server stores MK + K_aut + K_encr in Redis and issues a re-auth identity (pseudonym) as the TS.43 token. On subsequent requests, the device presents this identity, and the server
  conducts a lightweight EAP-AKA-Reauthentication exchange (subtype 13) — no HSS call, fresh session keys derived from the original MK.

 New Flow

 Full Auth (unchanged):  IMSI → 401 + AKA-Challenge → eap_relay → 200 + re-auth identity
 Fast Re-auth (new):     re-auth-id → 401 + AKA-Reauthentication → eap_relay → 200 + new re-auth-id
 GET (read-only):        re-auth-id → validate in Redis → 200 (no EAP, no counter change)
 ODSA temp tokens:       unchanged (token service remains for generateTemporaryToken)

 ---
 # Step 1: Constants (src/config/constants.ts)

 - Add REAUTH_SENT to EAP_STATE
 - Add REAUTH_SESSION_TTL_SECONDS: 90 and MAX_REAUTH_COUNTER: 65535 to EAP_AKA

# Step 2: EAP Codec (src/auth/eapCodec.ts)

 Add to AKA_SUBTYPE: REAUTHENTICATION: 13

 Add to AT: AT_PADDING: 6, AT_NEXT_REAUTH_ID: 14, AT_COUNTER: 19, AT_COUNTER_TOO_SMALL: 20, AT_NONCE_S: 21, AT_IV: 129, AT_ENCR_DATA: 130

 Add encode/decode cases:
 - AT_IV, AT_NONCE_S: Same layout as AT_RAND (2 reserved + 16 bytes value, length=5)
 - AT_COUNTER: Type(1) + Length=1(1) + Counter(2 bytes big-endian)
 - AT_COUNTER_TOO_SMALL: Type(1) + Length=1(1) + Reserved(2), no value
 - AT_PADDING: Type(1) + Length(1) + zero bytes to fill
 - AT_ENCR_DATA: 2 reserved bytes + variable-length ciphertext
 - AT_NEXT_REAUTH_ID: 2-byte actual-length prefix + UTF-8 identity + padding to 4-byte boundary

 Export encodeAttribute (currently private) so eapEncryption.ts can serialize inner attributes.

# Step 3: Encryption Module (NEW: src/auth/eapEncryption.ts)

 AES-128-CBC encrypt/decrypt for AT_ENCR_DATA per RFC 4187 Section 10.12.

 export function encryptAttributes(kEncr: Buffer, iv: Buffer, innerAttributes: EapAttribute[]): Buffer
 export function decryptAttributes(kEncr: Buffer, iv: Buffer, ciphertext: Buffer): EapAttribute[]

 - Serialize inner attributes via encodeAttribute(), append AT_PADDING if total isn't 16-byte aligned
 - AES-128-CBC with setAutoPadding(false) (we handle padding ourselves via AT_PADDING)
 - Decryption strips AT_PADDING from parsed attributes

# Step 4: Re-auth Key Derivation (src/auth/keyDerivation.ts)

 Add per RFC 4187 Section 7:

 export function deriveReauthKeys(identity: string, counter: number, nonceS: Buffer, mk: Buffer): { msk: Buffer, emsk: Buffer }

 - XKEY' = SHA-1(Identity | counter_as_2_byte_BE | NONCE_S | MK)
 - PRF(XKEY', 128) → MSK'(64) | EMSK'(64)
 - Reuses existing prfSha1()

 Note: K_aut and K_encr are NOT re-derived — they persist from the original full auth.

# Step 5: Re-auth State Store (NEW: src/auth/reauthStore.ts)

 Long-lived Redis storage keyed by re-auth identity. TTL = config.fastAuthTokenTtlSeconds (default 48h).

 interface ReauthState { subscriberId, imsi, mk, kAut, kEncr, counter, identity } // base64 for Buffers

 export function generateReauthId(): string           // crypto.randomBytes(24).toString('base64url')
 export function storeReauthState(state): Promise<void>
 export function getReauthState(reauthId): Promise<ReauthState | null>
 export function deleteReauthState(reauthId): Promise<void>

 Redis key pattern: reauth:{reauthId}

 Redis-only (no Postgres) — if lost, device falls back to full auth. No business state is at risk.

# Step 6: Re-auth Session Store (NEW: src/auth/reauthSession.ts)

 Short-lived session for the re-auth challenge/response correlation (90s TTL).

 interface ReauthSessionData {
   reauthId, nextReauthId, nonceS, counter, identifier,
   kAut, kEncr, mk, subscriberId, imsi
 }

 export function createReauthSession(data): Promise<string>  // returns sessionId
 export function getReauthSession(sessionId): Promise<ReauthSessionData | null>
 export function deleteReauthSession(sessionId): Promise<void>

 Redis key pattern: reauth_session:{sessionId} (distinct from eap_session: for full auth)

# Step 7: Re-auth Orchestrator (NEW: src/auth/eapReauth.ts)

 Two exported functions:

 handleReauthRequest(reauthId, clientIp) → ReauthChallengeResult

 1. getReauthState(reauthId) — if not found, return null (caller returns 401)
 2. Check counter < MAX_REAUTH_COUNTER — if exceeded, delete state, return null
 3. Generate nonceS (16 random bytes), nextReauthId, iv (16 random bytes)
 4. Build inner attributes: AT_COUNTER(counter), AT_NONCE_S(nonceS), AT_NEXT_REAUTH_ID(nextReauthId)
 5. encryptAttributes(kEncr, iv, innerAttributes) → ciphertext
 6. Build EAP-Request/AKA-Reauthentication: AT_IV(iv), AT_ENCR_DATA(ciphertext), AT_MAC(zeroed)
 7. Compute MAC with K_aut, patch into packet
 8. Store re-auth session (90s TTL) with all state needed for RT2
 9. Return { statusCode: 401, eapRelay, sessionId }

 handleReauthResponse(eapRelayBase64, sessionId, clientIp) → ReauthResult

 1. getReauthSession(sessionId) — if not found, return failure
 2. Decode EAP-Response, verify subtype = REAUTHENTICATION
 3. If subtype = AUTH_REJECT → delete session + state → failure
 4. Extract AT_IV, AT_ENCR_DATA, AT_MAC
 5. Verify AT_MAC with K_aut (BEFORE decryption — MAC covers ciphertext)
 6. Decrypt AT_ENCR_DATA with K_encr + IV
 7. Check for AT_COUNTER_TOO_SMALL:
   - If present → delete re-auth state → initiate full auth via handleInitialRequest(imsi) → return 401 + full-auth challenge + new sessionId (per RFC 4187 Section 5.4)
 8. Extract AT_COUNTER, verify it matches expected counter
 9. Derive new MSK'/EMSK' via deriveReauthKeys(nextReauthId, counter, nonceS, mk)
 10. Delete old re-auth state, store new state (nextReauthId, counter+1, same MK/K_aut/K_encr)
 11. Delete re-auth session
 12. Return { statusCode: 200, reauthId: nextReauthId, subscriberId, eapRelay: EAP-Success }

 Return types:
 interface ReauthChallengeResult { statusCode: 401, eapRelay: string, sessionId: string }
 interface ReauthResult { statusCode: number, reauthId?: string, eapRelay?: string, subscriberId?: string, sessionId?: string }

 The sessionId field in ReauthResult is set when counter-too-small triggers a full-auth fallback.

# Step 8: Modify Full Auth (src/auth/eapAka.ts + src/auth/eapSession.ts)

 eapSession.ts

 - Add mk: string (base64) field to EapSessionData
 - Update createSession() to accept and store mk: Buffer

 eapAka.ts — handleInitialRequest()

 - After deriveKeys(), also compute mk = deriveMasterKey(identity, vectors.ik, vectors.ck)
 - Pass mk to createSession() (new field)

 eapAka.ts — handleEapResponse()

 - Replace generateToken(subscriberId, TOKEN_TYPES.AUTH, clientIp) with:
 const reauthId = generateReauthId();
 await storeReauthState({
   subscriberId, imsi: session.imsi,
   mk: session.mk, kAut: session.kAut, kEncr: session.kEncr,
   counter: 1, identity: reauthId,
 });
 - Return token: reauthId in AuthResult (same field, different value)
 - Remove generateToken import, add generateReauthId, storeReauthState imports

# Step 9: Route Handler (src/server/routes/entitlement.ts)

 POST /entitlement — new 4-path routing:

 Path 1: eap_relay present
   a. getReauthSession(sessionId) → if found → handleReauthResponse()
      - If result.statusCode === 200: build entitlement response with result.reauthId as token
        * Handle AcquireTemporaryToken side-effect (existing ODSA logic)
      - If result.statusCode === 401 + result.sessionId: counter-too-small fallback
        → return 401 + result.eapRelay + new X-EAP-Session-Id
      - Else: return 401 + EAP-Failure
   b. Else → handleEapResponse() (full auth RT2, existing code)
      * Response now returns reauthId as token (no code change needed — same field)

 Path 2: token present
   a. getReauthState(token) → if found → handleReauthRequest(token) → return 401 + challenge
   b. Else → validateToken(token) → if valid → ODSA temporary token flow (existing)
   c. Else → 401 invalid

 Path 3: neither → handleInitialRequest(imsi) (existing full auth RT1)

 GET /entitlement:

 a. getReauthState(token) → if found → buildEntitlementResponse(token, state.subscriberId, ...)
 b. Else → validateToken(token) → existing flow (ODSA tokens)
 c. Else → 401

 Imports to add:

 - handleReauthRequest, handleReauthResponse from ./eapReauth.js
 - getReauthState from ./reauthStore.js
 - getReauthSession from ./reauthSession.js

 Imports to remove:

 - rotateToken (no longer used in main flow — ODSA doesn't rotate, only generates temp tokens)

# Step 10: Token Service Cleanup (src/auth/tokenService.ts)

 - generateToken() — KEEP (still used by generateTemporaryToken)
 - validateToken() — KEEP (still used for ODSA temporary tokens in route handler)
 - revokeToken() — KEEP (used internally)
 - rotateToken() — can be removed if nothing else imports it; check ODSA flow
 - generateTemporaryToken() — KEEP
 - findSubscriberByImsi() — KEEP

# Step 11: Tests

 Unit tests to add/update:

 - tests/unit/eapCodec.test.ts — encode/decode for all 7 new attribute types + REAUTHENTICATION subtype
 - tests/unit/eapEncryption.test.ts (NEW) — encrypt/decrypt round-trip, padding verification
 - tests/unit/keyDerivation.test.ts — deriveReauthKeys deterministic output, sensitivity to counter/nonce/identity

 Integration test helper:

 async function createTestReauthState(subscriberId: string, imsi: string) {
   const mk = crypto.randomBytes(20);
   const kAut = crypto.randomBytes(16);
   const kEncr = crypto.randomBytes(16);
   const reauthId = generateReauthId();
   await storeReauthState({ subscriberId, imsi, mk: mk.toString('base64'), kAut: kAut.toString('base64'), kEncr: kEncr.toString('base64'), counter: 1, identity: reauthId });
   return { reauthId, kAut, kEncr, mk, counter: 1 };
 }

 Plus a helper buildReauthResponse(challenge, kAut, kEncr) that:
 1. Decodes the EAP-Request/AKA-Reauthentication
 2. Decrypts AT_ENCR_DATA to get AT_COUNTER
 3. Builds EAP-Response/AKA-Reauthentication with encrypted AT_COUNTER + AT_MAC
 4. Returns base64-encoded eap_relay

 Integration test scenarios (tests/integration/eapAka.integration.test.ts):

 1. Full auth → re-auth identity: RT1 → RT2 → response has token (re-auth identity), not an opaque DB token
 2. Fast re-auth happy path: createTestReauthState → POST token → 401 + challenge → build response → POST eap_relay → 200 + new re-auth-id + entitlements
 3. Re-auth identity rotation: Old re-auth-id becomes invalid after successful re-auth, new one works
 4. Expired re-auth state: POST with stale re-auth-id → 401 (no re-auth state found)
 5. GET with re-auth identity: GET → 200 + entitlements (read-only, no counter change)

 ODSA tests (tests/integration/odsa.integration.test.ts):

 - Update getTokenForSubscriber() to use createTestReauthState() instead of generateToken()
 - For AcquireTemporaryToken, either:
   - Use the full re-auth exchange via test helpers, OR
   - Keep a simpler approach: create re-auth state, perform re-auth via helpers, then test ODSA

 ---
 Files Summary

 New files (4):
 - src/auth/eapEncryption.ts
 - src/auth/reauthStore.ts
 - src/auth/reauthSession.ts
 - src/auth/eapReauth.ts
 - tests/unit/eapEncryption.test.ts

 Modified files (8):
 - src/config/constants.ts
 - src/auth/eapCodec.ts
 - src/auth/keyDerivation.ts
 - src/auth/eapSession.ts
 - src/auth/eapAka.ts
 - src/auth/tokenService.ts (minimal — just cleanup unused imports if any)
 - src/server/routes/entitlement.ts
 - tests/integration/eapAka.integration.test.ts
 - tests/integration/odsa.integration.test.ts
 - tests/unit/eapCodec.test.ts
 - tests/unit/keyDerivation.test.ts

 Verification

 1. npm run build — TypeScript compilation passes
 2. npm run test — all unit tests pass (new + existing)
 3. docker compose up -d postgres redis mock-hss then npm run test:integration — all integration tests pass
 4. Manual verification: full auth → re-auth → GET → expired re-auth → full auth again
╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌