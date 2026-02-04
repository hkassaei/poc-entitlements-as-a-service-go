# Phase 3: EAP-AKA Authentication

## Overview
Implement the EAP-AKA protocol over HTTP — the core authentication mechanism that proves a device has a valid SIM. This replaces the 501 stubs in the entitlement route with a working two-round-trip challenge/response flow. 7 new files + 1 modified file in `src/`, plus 3 test files.

## Authentication Flow

**Round Trip 1 — Challenge:**
1. Device POSTs `/entitlement` with IMSI (no token, no eap_relay)
2. ECS calls mock-hss `POST /vectors {imsi}` → gets {rand, autn, xres, ck, ik}
3. ECS derives keys: MK = SHA-1(Identity | IK | CK), then K_encr, K_aut via FIPS 186-2 PRF
4. ECS stores session in Redis (state=CHALLENGE_SENT, 90s TTL)
5. ECS builds EAP-Request/AKA-Challenge with AT_RAND, AT_AUTN, AT_MAC
6. Returns 401 + eap_relay (base64) + `X-EAP-Session-Id` header

**Round Trip 2 — Response:**
1. Device POSTs with `eap_relay` (AT_RES, AT_MAC) + `X-EAP-Session-Id` header
2. ECS verifies AT_RES == stored XRES (timing-safe), verifies AT_MAC with K_aut
3. Generates auth token → Postgres + Redis cache
4. Caches response for idempotency (90s TTL)
5. Returns 200 + token

**Fast Auth:** Device POSTs with `token` → validated via Redis/Postgres → skip EAP-AKA

## Implementation Steps (dependency order)

### Step 1: EAP Packet Codec
**File:** `src/auth/eapCodec.ts`

Binary encode/decode of EAP-AKA packets per RFC 4187.

**Packet header** (8 bytes): `Code(1) | Identifier(1) | Length(2) | Type(1) | Subtype(1) | Reserved(2)`
- EAP-Success/Failure are special: only 4 bytes `Code(1) | Id(1) | Length(2)` — no Type/Subtype

**Attribute TLV**: `Type(1) | Length-in-4byte-words(1) | Value(padded to 4 bytes)`
- AT_RAND (1): 2 reserved + 16 bytes RAND, length=5
- AT_AUTN (2): 2 reserved + 16 bytes AUTN, length=5
- AT_RES (3): 2-byte bit-length prefix + value (padded), length=3 for 8-byte XRES
- AT_AUTS (4): 2 reserved + 14 bytes, length=4
- AT_MAC (11): 2 reserved + 16 bytes MAC, length=5

Constants: `EAP_CODE` (REQUEST=1, RESPONSE=2, SUCCESS=3, FAILURE=4), `EAP_TYPE_AKA=23`, `AKA_SUBTYPE` (CHALLENGE=1, AUTH_REJECT=2, SYNC_FAILURE=4, IDENTITY=5)

Exports: `encodeEapPacket()`, `decodeEapPacket()`, `encodeEapToBase64()`, `decodeEapFromBase64()`

### Step 2: Key Derivation
**File:** `src/auth/keyDerivation.ts`

RFC 4187 Section 7 key derivation using the FIPS 186-2 SHA-1-based PRF.

- `buildIdentity(imsi)`: returns `"0" + imsi` (type 0 = permanent identity)
- `deriveMasterKey(identity, ik, ck)`: `MK = SHA-1(Identity | IK | CK)` → 20 bytes
- `prf(mk, outputLength)`: FIPS 186-2 PRF — SHA-1 based with 160-bit modular arithmetic. Generates 160 bytes: `K_encr(16) | K_aut(16) | MSK(64) | EMSK(64)`
- `deriveKeys(identity, ik, ck)`: returns `{ kEncr, kAut, msk, emsk }`
- `computeMac(kAut, eapPacketWithZeroMac)`: HMAC-SHA-1 truncated to 16 bytes
- `verifyMac(kAut, rawPacketBytes, receivedMac)`: zeros the MAC field in packet bytes, recomputes, compares with timing-safe equal

**PRF algorithm** (each iteration):
1. `w = SHA-1(xkey)` — 20 bytes output
2. `xkey = (xkey + w + 1) mod 2^160` — big-endian 160-bit addition
3. Collect w, repeat until 160 bytes generated

### Step 3: HSS Vector Client
**File:** `src/auth/eapAkaVectors.ts`

Thin HTTP client using native `fetch()` (Node 20+):
- `fetchVectors(imsi)` → calls `${config.hssUrl}/vectors` with `{imsi}`
- Returns `{ rand, autn, xres, ck, ik }` as Buffers (decoded from base64)
- 5s timeout via `AbortSignal.timeout()`

### Step 4: EAP Session Manager
**File:** `src/auth/eapSession.ts`

Redis hash operations for EAP sessions:
- Key pattern: `eap_session:{sessionId}` with 90s TTL
- Session data: `{ imsi, state, rand, xres, ck, ik, identifier, kAut }` (all base64 strings)
- `createSession()`, `getSession()`, `updateSessionState()`, `storeSessionKeys()`, `deleteSession()`

### Step 5: Token Service
**File:** `src/auth/tokenService.ts`

Token generation and validation:
- `generateToken(subscriberId, tokenType, clientIp)`: `crypto.randomBytes(32).toString('hex')` → INSERT into Postgres `tokens` table + SET in Redis with TTL
- `validateToken(tokenValue)`: check Redis cache first, fallback to Postgres
- `findSubscriberByImsi(imsi)`: query subscribers table for subscriber ID

### Step 6: Idempotency Cache
**File:** `src/auth/eapIdempotency.ts`

Response replay cache in Redis:
- Key: `eap_idempotency:` + SHA-256(sessionId + eapRelay)
- `cacheResponse(sessionId, eapRelay, response)` — SET with 90s TTL
- `getCachedResponse(sessionId, eapRelay)` → cached response or null

### Step 7: EAP-AKA Orchestrator
**File:** `src/auth/eapAka.ts`

Central state machine tying Steps 1-6 together:
- `handleInitialRequest(imsi, clientIp)` → fetchVectors → deriveKeys → build EAP-Challenge with AT_MAC → createSession → return 401 + eapRelay + sessionId
- `handleEapResponse(eapRelayBase64, sessionId, clientIp)` → getSession → decode packet → verify AT_RES (timing-safe) → verify AT_MAC → generateToken → deleteSession → return 200 + token
- Handles edge cases: AUTH_REJECT, SYNC_FAILURE subtypes, missing attributes, expired sessions

### Step 8: Route Wiring
**File:** `src/server/routes/entitlement.ts` (modify existing)

Replace 501 stubs with three-path routing:
1. **`token` present** → `validateToken()` → 200 with entitlement data (or 401)
2. **`eap_relay` present** → check idempotency cache → `handleEapResponse()` → cache success → 200 (or 401)
3. **Neither** (initial) → require `imsi` → `handleInitialRequest()` → 401 with challenge

GET handler: require token, validate, return entitlement data.

### Step 9: Tests

**`tests/eapCodec.test.ts`** — Unit tests:
- Round-trip encode/decode for AKA-Challenge (AT_RAND + AT_AUTN + AT_MAC)
- Round-trip encode/decode for AKA-Response (AT_RES + AT_MAC)
- EAP-Success/Failure encode to 4 bytes
- AT_RES bit-length prefix encoding
- Malformed packet rejection

**`tests/keyDerivation.test.ts`** — Unit tests:
- MK derivation produces correct 20-byte SHA-1
- PRF produces deterministic output of correct length
- deriveKeys splits correctly into K_encr(16) + K_aut(16) + MSK(64) + EMSK(64)
- computeMac/verifyMac round-trip
- verifyMac rejects tampered packets

**`tests/eapAka.integration.test.ts`** — Integration tests (requires Docker):
- Full handshake: POST (initial) → 401 challenge → POST (EAP-Response) → 200 + token
- Token validation on subsequent request
- Idempotency: same Round Trip 2 twice → same cached response
- Unknown IMSI → 403
- Expired session → error
- Wrong AT_RES → 401

## Files Created/Modified (10 total)

```
src/auth/
  eapCodec.ts          (new) — EAP-AKA packet binary codec
  keyDerivation.ts     (new) — RFC 4187 key derivation + MAC
  eapAkaVectors.ts     (new) — HSS HTTP client
  eapSession.ts        (new) — Redis session management
  tokenService.ts      (new) — Token generation/validation
  eapIdempotency.ts    (new) — Response idempotency cache
  eapAka.ts            (new) — Orchestrator state machine

src/server/routes/
  entitlement.ts       (modify) — Replace 501 stubs with auth logic

tests/
  eapCodec.test.ts     (new) — Codec unit tests
  keyDerivation.test.ts (new) — Key derivation unit tests
  eapAka.integration.test.ts (new) — End-to-end integration tests
```

## Verification

1. `npm test` — All unit tests pass (codec round-trips, key derivation, MAC computation)
2. `npm run build` — TypeScript compiles cleanly
3. `docker compose up --build` — All services start
4. Run seed (if not already seeded)
5. Full handshake test:
   ```bash
   # RT1: Initial request
   curl -s -X POST http://localhost:8443/entitlement \
     -H 'Content-Type: application/json' \
     -d '{"app":"ap2004","terminal_id":"12345678901234","entitlement_version":"2","imsi":"001010000000001"}' \
     -D -
   # → 401 + eap_relay + X-EAP-Session-Id header

   # RT2: Simulated EAP response (integration test handles this programmatically)
   # → 200 + token
   ```
6. Token reuse: GET /entitlement with token → 200
7. Ki never appears in any response, log, or error message


## Integration test results

All 54 tests pass across all 5 test files:
┌──────────────────────────────────┬───────┬────────┐
│            Test File             │ Tests │ Status │
├──────────────────────────────────┼───────┼────────┤
│ tests/eapCodec.test.ts           │ 12    │ Pass   │
├──────────────────────────────────┼───────┼────────┤
│ tests/keyDerivation.test.ts      │ 13    │ Pass   │
├──────────────────────────────────┼───────┼────────┤
│ tests/eapAka.integration.test.ts │ 8     │ Pass   │
├──────────────────────────────────┼───────┼────────┤
│ mock-hss/tests/milenage.test.ts  │ 12    │ Pass   │
├──────────────────────────────────┼───────┼────────┤
│ mock-hss/tests/kms.test.ts       │ 9     │ Pass   │
└──────────────────────────────────┴───────┴────────┘

The integration tests confirmed: EAP-AKA challenge generation works end-to-end (ECS → mock HSS → MILENAGE → challenge packet), and all error paths (unknown IMSI → 403, missing IMSI → 400, expired session →
401, invalid token → 401, missing session header → 400) behave correctly.