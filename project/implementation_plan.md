# Entitlements-as-a-Service: Technical Implementation Plan

## 1. Technology Stack

| Layer | Choice | Rationale |
|-------|--------|-----------|
| Language | **TypeScript** | Strong typing for complex protocol structures; excellent ecosystem |
| Runtime | **Node.js 20+** | Async I/O fits HTTP server workload; native crypto support |
| HTTP Framework | **Fastify** | Schema-based validation, compiled serialization, plugin encapsulation, TypeScript-first |
| Schema Definition | **@sinclair/typebox** | TypeScript-native syntax that outputs native JSON Schema; single source of truth for both runtime validation and static types |
| Validation Engine | **Native Fastify (Ajv compiled)** | Fastify compiles TypeBox JSON Schemas into Ajv validators at startup — zero per-request interpretation overhead |
| Database | **PostgreSQL** | Relational model suits subscriber/entitlement data; JSONB for flexible config |
| ORM | **Drizzle ORM** | Type-safe, lightweight, SQL-close abstractions |
| Cache | **Redis** | Token store, session cache, fast auth token lookup |
| Testing | **Vitest** | Fast, TypeScript-native, compatible with Node APIs |
| Containerization | **Docker + Docker Compose** | Reproducible local dev with Postgres + Redis |
| Logging | **Pino** | Structured JSON logging, low overhead; auto-ingested by Cloud Logging |
| Tracing | **OpenTelemetry + Cloud Trace** | Distributed tracing across ECS → mock HSS; auto-instruments HTTP, pg, ioredis |
| Metrics | **OpenTelemetry + Cloud Monitoring** | Custom application metrics (auth rates, cache hits, latency); alerting policies |

---

## 2. Project Structure

```
src/
  index.ts                      # Server entrypoint
  config/
    index.ts                    # Environment config loader
    constants.ts                # AppIDs, status codes, protocol constants
    logger.ts                   # Pino setup with Cloud Logging severity + trace correlation
    tracing.ts                  # OpenTelemetry SDK + Cloud Trace exporter (initialized before Fastify)
    metrics.ts                  # OpenTelemetry Metrics + Cloud Monitoring exporter
  server/
    app.ts                      # Fastify app setup, global hooks and plugins
    routes/
      entitlement.ts            # Main /entitlement endpoint (GET + POST)
      auth.ts                   # OAuth/OIDC callback routes
      notification.ts           # Push notification registration
    middleware/
      requestParser.ts          # Normalize GET query / POST JSON into unified request
      userAgent.ts              # Parse and validate TS.43 User-Agent header
      errorHandler.ts           # Centralized HTTP error responses
      versionCheck.ts           # entitlement_version negotiation (406 if unsupported)
  auth/
    eapAka.ts                   # EAP-AKA challenge/response state machine
    eapIdempotency.ts           # Cached response replay for EAP-AKA retries
    eapAkaCodec.ts              # EAP packet encode/decode (RFC 4187)
    eapAkaVectors.ts            # AKA vector client — calls HSS_URL for vectors (mock or real)
    tokenManager.ts             # Issue, validate, refresh auth tokens + fast-auth tokens
    temporaryToken.ts           # Scoped temporary tokens for third-party delegation
    oauthClient.ts              # OAuth 2.0 / OIDC redirect + token exchange
  services/
    entitlementService.ts       # Core orchestrator: route request to correct handler
    vowifi.ts                   # ap2004 — VoWiFi entitlement logic
    vowifiConfig.ts             # VoWiFi provisioning document builder
    voiceOverCellular.ts        # ap2003 — VoLTE/VoNR logic
    smsOverIp.ts                # ap2005 — SMSoIP logic
    odsaCompanion.ts            # ap2006 — Companion device ODSA
    odsaPrimary.ts              # ap2009 — Primary device ODSA
    dataPlan.ts                 # ap2010 — Data plan info, boost, usage
    serverOdsa.ts               # ap2011 — Server-initiated ODSA (enterprise)
    directCarrierBilling.ts     # ap2012 — DCB entitlement
    privateUserIdentity.ts      # ap2013 — Pseudonym/encrypted IMSI
    deviceUserInfo.ts           # ap2014 — MSISDN and subscriber info
    appAuthentication.ts        # ap2015 — OperatorToken / AppToken
    satMode.ts                  # ap2016 — Satellite connectivity
  protocol/
    responseBuilder.ts          # Build XML (WAP-Provisioning) and JSON responses
    xmlBuilder.ts               # WAP-Provisioning XML serializer
    jsonBuilder.ts              # JSON response serializer
    requestSchemas.ts           # JSON Schemas for runtime validation (Fastify/Ajv)
    requestTypes.ts             # TypeScript types inferred from schemas (compile-time)
    responseTypes.ts            # TypeScript types for all response structures
    appIds.ts                   # AppID enum + lookup table
    statusCodes.ts              # EntitlementStatus, ServiceStatus, SubscriptionResult enums
  db/
    schema.ts                   # Drizzle schema definitions
    migrations/                 # SQL migration files
    subscribers.ts              # Subscriber CRUD (IMSI, MSISDN, ICCID, EID)
    entitlements.ts             # Per-subscriber entitlement records
    tokens.ts                   # Token persistence (fast-auth, temporary)
    auditLog.ts                 # Request/response audit trail
  mock/
    subscribers.ts              # Seed subscriber data (Ki encrypted at insert time via mock HSS)
    bssBackend.ts               # Mock BSS/OSS responses
    smdpPlus.ts                 # Mock SM-DP+ profile server
  utils/
    crypto.ts                   # HMAC, SHA-256, key derivation helpers
mock-hss/                         # SEPARATE SERVICE — internal only, never internet-facing
  src/
    index.ts                    # Fastify server, single POST /vectors endpoint
    milenage.ts                 # MILENAGE algorithm implementation
    kms.ts                      # Cloud KMS envelope encryption (unwrap DEK → decrypt Ki)
    db.ts                       # Read encrypted Ki/OP from subscribers table
  Dockerfile
  package.json
  tsconfig.json
    base64.ts                   # Base64 encode/decode with encodedValue= prefix
    phone.ts                    # E.164 phone number validation
    retry.ts                    # Retry-After header helper
tests/
  unit/
    auth/
    services/
    protocol/
  integration/
    eapAkaFlow.test.ts
    entitlementFlow.test.ts
    odsaFlow.test.ts
  fixtures/
    akaVectors.json
    sampleRequests.json
    sampleResponses.json
docker-compose.yml
Dockerfile
tsconfig.json
package.json
```

---

## 3. EAP-AKA Authentication (Core Protocol)

This is the most complex component. EAP-AKA authenticates devices using cryptographic challenges derived from SIM card secrets — without the server ever seeing the secret key (Ki). The authentication is "embedded" inside HTTP request/response cycles per TS.43 Section 4.

### 3.1 Protocol Flow (HTTP-Embedded EAP-AKA)

```
Device                              ECS                         Mock HSS (internal)
  |                                  |                              |
  |-- POST /entitlement ------------>|                              |
  |   (IMEI, app, terminal_*)       |                              |
  |                                  |-- POST /vectors {imsi,sqn} ->|
  |                                  |   (internal VPC only)        |
  |                                  |<-- RAND, AUTN, XRES, CK, IK |
  |                                  |   (Ki never leaves mock HSS) |
  |<-- 401 + EAP-Request/AKA -------|                              |
  |   (AT_RAND, AT_AUTN, AT_MAC)    |                              |
  |                                  |                              |
  |-- POST /entitlement ------------>|                              |
  |   (EAP-Response/AKA: AT_RES,    |                              |
  |    AT_MAC)                       |                              |
  |                                  |-- Verify RES == XRES         |
  |                                  |-- Derive MSK from CK, IK     |
  |                                  |                              |
  |<-- 200 OK + Token + Config ------|                              |
```

### 3.2 EAP-AKA State Machine

```
States:
  IDLE            → Waiting for initial request
  CHALLENGE_SENT  → AKA-Challenge sent to device, awaiting response
  SUCCESS         → RES verified, session authenticated
  FAILURE         → Authentication failed (bad RES, sync failure, etc.)
  SYNC_FAILURE    → SQN out of sync, re-sync required (AT_AUTS handling)

Transitions:
  IDLE → CHALLENGE_SENT:
    Trigger: Receive initial request (no token / expired token)
    Action: Generate AKA vectors, build EAP-Request/AKA-Challenge, respond 401

  CHALLENGE_SENT → SUCCESS:
    Trigger: Receive EAP-Response with valid AT_RES (RES == XRES) and valid AT_MAC
    Action: Derive MSK, issue auth token, cache response (see 3.2.1), return entitlement config

  CHALLENGE_SENT → FAILURE:
    Trigger: RES != XRES, or invalid AT_MAC
    Action: Return EAP-Failure, respond 403

  CHALLENGE_SENT → SYNC_FAILURE:
    Trigger: Device sends AT_AUTS (sequence number mismatch)
    Action: Forward AUTS to HSS for re-sync, get new vectors, re-challenge

  ANY → IDLE:
    Trigger: Timeout (session expires without completion)
```

### 3.2.1 EAP-AKA Response Idempotency

**The problem:** A device sends EAP-Response/AKA, the server verifies RES, issues a
token, and returns 200 OK with the entitlement config. But the network drops the
response. The device retries the same POST. By now the server has already transitioned
the session to SUCCESS (or deleted it). Without special handling, the server either:
- Sees no session → starts a new EAP-AKA challenge (unnecessary full re-auth)
- Sees a SUCCESS session → returns an error (wrong)

Both outcomes are incorrect. The device already authenticated — it just needs the
response it missed.

**The fix: cached response replay**

On successful EAP-AKA completion, before returning the 200 OK, the server caches the
complete HTTP response body in Redis, keyed by a fingerprint of the request. If the
same request arrives again, the cached response is replayed verbatim.

```
First request (EAP-Response/AKA):

  1. Validate RES == XRES, verify AT_MAC              ✓
  2. Derive MSK, issue auth token
  3. Build 200 OK response (token + entitlement config)
  4. Compute request fingerprint: SHA-256(session_id + eap_relay)
  5. Cache in Redis:
       idempotent:{fingerprint} → { status: 200, body: <complete response> }
       TTL: 90 seconds (same as session TTL — retry window)
  6. Transition session to SUCCESS
  7. Return 200 OK to device


Retry (same EAP-Response/AKA, network dropped previous response):

  1. Compute request fingerprint: SHA-256(session_id + eap_relay)
  2. Check Redis: GET idempotent:{fingerprint}
  3. HIT → Return cached response verbatim (same token, same config)
  4. Device receives the response it missed — no re-authentication needed
```

**Redis key schema:**

```
idempotent:{sha256_fingerprint} → Hash {
  status_code: 200,
  headers: { content-type, ... },
  body: <serialized response>
}
TTL: 90 seconds
```

**Why 90 seconds?** Matches the EAP-AKA session TTL. If the device hasn't retried
within 90 seconds, it will start a fresh authentication anyway.

**What the fingerprint covers:**

The fingerprint must uniquely identify "the same logical request":
- `session_id` — ties to the specific EAP-AKA session (prevents cross-session collisions)
- `eap_relay` — the exact EAP-Response payload (AT_RES, AT_MAC)

This ensures only identical retries get the cached response. A different device,
a different session, or a different EAP payload will miss the cache and go through
normal processing.

**Edge cases:**

| Scenario | Behavior |
|----------|----------|
| Retry arrives while first request is still processing | Session is still in CHALLENGE_SENT — normal processing runs; second response wins, first may cache over it (same result since inputs are identical) |
| Retry after session expired (>90s) | Cache key also expired — device gets a new EAP-AKA challenge (correct behavior) |
| Attacker replays a captured EAP-Response | Session already consumed, cache expired — returns 401 new challenge (correct) |
| Device retries 3+ times within 90s | All retries hit the cache — same response every time |

### 3.3 EAP Packet Structure (eapAkaCodec.ts)

Implementation must handle these EAP-AKA attribute types (RFC 4187):

| Attribute | Code | Size | Description |
|-----------|------|------|-------------|
| AT_RAND | 1 | 20 bytes | Random challenge from network |
| AT_AUTN | 2 | 16 bytes | Authentication token (proves network identity) |
| AT_RES | 3 | 4-16 bytes | Device response (proves SIM identity) |
| AT_AUTS | 4 | 14 bytes | Re-sync token (SQN mismatch) |
| AT_MAC | 11 | 16 bytes | Message authentication code |
| AT_IV | 129 | 16 bytes | Initialization vector |
| AT_ENCR_DATA | 130 | variable | Encrypted data |
| AT_CHECKCODE | 134 | 0 or 32 bytes | Hash of EAP messages exchanged |

Key derivation (RFC 4187 Section 7):
```
MK = SHA1(Identity | IK | CK)
K_encr = MK[0..15]      (encryption key)
K_aut  = MK[16..31]     (authentication/MAC key)
MSK    = MK[32..95]     (master session key)
EMSK   = MK[96..159]    (extended master session key)
```

### 3.4 AKA Vector Generation (Mock for POC)

The ECS calls `POST ${HSS_URL}/vectors` to obtain AKA vectors (see Section 8). The ECS
never sees Ki — it only receives the derived vectors. The ECS code has zero awareness
of what's behind that URL. Swapping from mock HSS to a real HSS (or a Diameter-to-HTTP
gateway fronting a real HSS) is a config change — set `HSS_URL` to the new endpoint.
No code changes, no redeployment of the ECS image.

### 3.5 MILENAGE Implementation (inside mock HSS — `mock-hss/src/milenage.ts`)

We implement MILENAGE ourselves in TypeScript rather than using the existing `milenage`
npm package (7 stars, no TypeScript types, no security audit, unmaintained). The
algorithm is well-specified in 3GPP TS 35.206 and the only cryptographic primitive is
AES-128-ECB, which we delegate entirely to Node.js `crypto` (backed by OpenSSL) — we
do not implement any cryptographic primitives ourselves.

**Algorithm structure (per 3GPP TS 35.206):**

MILENAGE defines 7 functions, all built on a single core operation: AES-128 encryption
of a 128-bit block XOR'd with constants and rotated by offsets.

```
Core operation:
  OPc = AES_K(OP) ⊕ OP                    (computed once per subscriber)

  TEMP = AES_K(RAND ⊕ OPc)                 (computed once per authentication)

Functions:
  f1 (MAC-A):    AES_K(rot(TEMP ⊕ OPc, r1) ⊕ c1 ⊕ (SQN||AMF||SQN||AMF)) ⊕ OPc
  f1*(MAC-S):    AES_K(rot(TEMP ⊕ OPc, r1) ⊕ c1 ⊕ (SQN||AMF||SQN||AMF)) ⊕ OPc
                 (same as f1 but with different AMF padding)
  f2 (RES):      [AES_K(rot(TEMP ⊕ OPc, r2) ⊕ c2) ⊕ OPc] truncated to 64 bits
  f3 (CK):       AES_K(rot(TEMP ⊕ OPc, r3) ⊕ c3) ⊕ OPc
  f4 (IK):       AES_K(rot(TEMP ⊕ OPc, r4) ⊕ c4) ⊕ OPc
  f5 (AK):       [AES_K(rot(TEMP ⊕ OPc, r5) ⊕ c5) ⊕ OPc] truncated to 48 bits
  f5*(AK):       same structure with different rotation/constant

Where:
  AES_K()  = AES-128-ECB encrypt with key K (Node.js crypto.createCipheriv)
  rot(x,r) = byte rotation of x by r positions
  c1..c5   = fixed 128-bit constants defined in the spec
  r1..r5   = fixed rotation offsets defined in the spec
```

**What we implement vs. what we delegate:**

| Component | Our code | Delegated to |
|-----------|----------|--------------|
| AES-128-ECB encryption | No | Node.js `crypto` → OpenSSL (hardware-accelerated) |
| XOR of 128-bit blocks | Yes | — (trivial bitwise operation) |
| Byte rotation by offset | Yes | — (array slice + concat) |
| OPc derivation | Yes (calls AES) | AES via `crypto` |
| f1, f1*, f2, f3, f4, f5, f5* | Yes (calls AES) | AES via `crypto` |
| AUTN construction (SQN⊕AK ‖ AMF ‖ MAC-A) | Yes | — (byte concatenation) |
| RAND generation | No | `crypto.randomBytes(16)` |

**Implementation skeleton:**

```typescript
// mock-hss/src/milenage.ts
import { createCipheriv } from 'node:crypto';

// AES-128-ECB: the only crypto primitive we need
function aes128Encrypt(key: Buffer, input: Buffer): Buffer {
  const cipher = createCipheriv('aes-128-ecb', key, null);
  cipher.setAutoPadding(false);
  return Buffer.concat([cipher.update(input), cipher.final()]);
}

// XOR two 16-byte buffers
function xor(a: Buffer, b: Buffer): Buffer { ... }

// Rotate buffer by r bytes
function rotate(buf: Buffer, r: number): Buffer { ... }

// Derive OPc from OP and Ki (done once per subscriber, can be cached)
function computeOPc(ki: Buffer, op: Buffer): Buffer {
  return xor(aes128Encrypt(ki, op), op);
}

// Core MILENAGE functions
export function f1(ki: Buffer, opc: Buffer, rand: Buffer, sqn: Buffer, amf: Buffer): Buffer { ... }  // MAC-A
export function f2345(ki: Buffer, opc: Buffer, rand: Buffer): { res: Buffer; ck: Buffer; ik: Buffer; ak: Buffer } { ... }
export function f1star(ki: Buffer, opc: Buffer, rand: Buffer, sqn: Buffer, amf: Buffer): Buffer { ... }  // MAC-S (for re-sync)
export function f5star(ki: Buffer, opc: Buffer, rand: Buffer): Buffer { ... }  // AK for re-sync

// High-level: generate a complete AKA vector set
export function generateVectors(ki: Buffer, op: Buffer, rand: Buffer, sqn: Buffer, amf: Buffer) {
  const opc = computeOPc(ki, op);
  const { res, ck, ik, ak } = f2345(ki, opc, rand);
  const macA = f1(ki, opc, rand, sqn, amf);
  const autn = Buffer.concat([xor(sqn, ak), amf, macA]);  // SQN⊕AK || AMF || MAC-A
  return { rand, autn, xres: res, ck, ik };
}
```

**Validation against official 3GPP test vectors:**

3GPP TS 35.207 and TS 35.208 provide official test vectors — known Ki, OP, RAND, SQN,
AMF inputs with expected outputs for every function. We use these as unit tests:

```typescript
// mock-hss/tests/milenage.test.ts
// Test Set 1 from 3GPP TS 35.207
test('MILENAGE test set 1', () => {
  const ki   = Buffer.from('465b5ce8b199b49faa5f0a2ee238a6bc', 'hex');
  const op   = Buffer.from('cdc202d5123e20f62b6d676ac72cb318', 'hex');
  const rand = Buffer.from('23553cbe9637a89d218ae64dae47bf35', 'hex');
  const sqn  = Buffer.from('ff9bb4d0b607', 'hex');
  const amf  = Buffer.from('b9b9', 'hex');

  const vectors = generateVectors(ki, op, rand, sqn, amf);

  expect(vectors.xres.toString('hex')).toBe('a54211d5e3ba50bf');
  expect(vectors.ck.toString('hex')).toBe('b40ba9a3c58b2a05bbf0d987b21bf8cb');
  expect(vectors.ik.toString('hex')).toBe('f769bcd751044604127672711c6d3441');
  // ... etc for all outputs
});
```

If our implementation passes all official test sets, it is provably correct — there is
no ambiguity in the algorithm.

**Inputs and outputs (complete flow inside mock HSS):**

```
Inputs:
  Ki    — 128-bit subscriber secret key (decrypted from envelope encryption, zeroed after use)
  OP    — 128-bit operator variant key (decrypted alongside Ki)
  SQN   — 48-bit sequence number (from subscribers table, incremented after each auth)
  AMF   — 16-bit authentication management field (operator-configured constant)
  RAND  — 128-bit random challenge (generated fresh via crypto.randomBytes)

Outputs (returned to ECS via POST /vectors response):
  RAND  — the random challenge (ECS forwards to device)
  AUTN  — authentication token = SQN⊕AK || AMF || MAC-A (proves network identity to device)
  XRES  — expected response (ECS compares with device's RES to verify SIM identity)
  CK    — cipher key (128-bit, used for key derivation)
  IK    — integrity key (128-bit, used for key derivation)
```

---

## 4. Token Management

### 4.1 Token Types & Hierarchy

```
EAP-AKA Authentication
        |
        v
   Auth Token (long-lived, ~24-48h)
        |
        v
   Fast-Auth Token (short-lived, device presents on subsequent requests)
        |
        ├──> Temporary Token (scoped, delegated to third parties)
        └──> Operator Token (encrypted, for app authentication)
```

### 4.2 Token Schema

```sql
CREATE TABLE tokens (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscriber_id   UUID NOT NULL REFERENCES subscribers(id),
    token_value     VARCHAR(512) NOT NULL UNIQUE,
    token_type      VARCHAR(32) NOT NULL,   -- 'auth', 'fast_auth', 'temporary', 'operator'
    issued_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL,
    scope           VARCHAR(256),            -- for temporary tokens: allowed operations
    operation_targets TEXT[],                -- for temporary tokens: allowed target params
    consumed        BOOLEAN DEFAULT FALSE,   -- for one-time tokens
    created_by_ip   INET
);
```

### 4.3 Token Storage: Write-Through Cache Pattern

Long-lived tokens (auth, fast-auth) use a **write-through cache** with Postgres as the
durable source of truth and Redis as the fast-path lookup layer.

**Why not Redis-only?** Unlike EAP-AKA sessions (ephemeral, seconds), auth tokens live
24-48 hours. If Redis restarts or evicts keys, every device would need a full EAP-AKA
re-authentication. Postgres guarantees tokens survive infrastructure events.

**Why not Postgres-only?** Token validation is the hottest read path in the system — every
single request hits it. Sub-millisecond Redis lookups keep latency low and keep
Postgres free for actual data queries.

```
Token Write (after successful EAP-AKA):

  EAP-AKA SUCCESS
      |
      v
  Generate token (cryptographically random)
      |
      ├──> INSERT into Postgres tokens table (durable record)
      ├──> SET in Redis with TTL matching token expiry (fast-path cache)
      └──> Return token to device in HTTP 200 response

  Redis key schema:
    token:{token_value} → Hash {
      subscriber_id, token_type, expires_at, scope, operation_targets
    }
    TTL: matches token expiry (AUTH_TOKEN_TTL_SECONDS or FAST_AUTH_TOKEN_TTL_SECONDS)


Token Read (on every subsequent request):

  Request arrives with token
      |
      v
  Check Redis: GET token:{token_value}
      |
      ├── HIT → Token valid, subscriber identified, skip EAP-AKA
      |
      └── MISS → Query Postgres: SELECT * FROM tokens WHERE token_value = ?
            |
            ├── Found + not expired → Re-populate Redis cache, proceed
            └── Not found or expired → 401, start EAP-AKA challenge


Token Revocation:

  Revoke token
      |
      ├──> DELETE from Redis (immediate effect on hot path)
      └──> UPDATE consumed=true in Postgres (audit trail)
```

### 4.4 Fast Authentication Flow

```
Device                              ECS
  |                                  |
  |-- POST /entitlement ------------>|
  |   (token=<fast_auth_token>)      |
  |                                  |-- Redis lookup: token:{value}
  |                                  |-- HIT → subscriber identified
  |<-- 200 OK + new token + config --|
  |                                  |
  (No EAP-AKA round-trips needed)
```

When a valid fast-auth token is presented, the server skips authentication entirely and returns the entitlement configuration directly. A new token is issued with each response to implement rolling expiry. The old token is revoked (deleted from Redis, marked consumed in Postgres) and the new token is written through to both stores.

---

## 5. Request Processing Pipeline

### 5.1 Unified Request Model

Both GET (query string) and POST (JSON body) requests carry the same parameters. Fastify's JSON Schema validation handles POST body parsing and type coercion natively. A `preHandler` hook normalizes GET query params into the same shape:

```typescript
interface EntitlementRequest {
  // Device identification
  app: string;                    // AppID (e.g., "ap2004")
  terminal_id: string;            // IMEI
  terminal_vendor: string;
  terminal_model: string;
  terminal_sw_version: string;
  terminal_type?: string;         // "Smartwatch", "Tablet", etc.

  // Authentication
  token?: string;                 // Fast-auth or temporary token
  eap_relay?: string;             // Base64-encoded EAP response from device

  // Entitlement
  entitlement_version: string;    // Protocol version ("2", "4", etc.)
  IMSI?: string;
  MSISDN?: string;
  EID?: string;
  ICCID?: string;

  // ODSA-specific
  operation?: string;             // "CheckEligibility", "ManageSubscription", etc.
  operation_type?: number;        // Sub-operation code
  companion_terminal_id?: string;
  companion_terminal_EID?: string;
  target_ICCID?: string;

  // Response format
  accept_content_type?: string;   // "xml" or "json"
}
```

### 5.2 Processing Pipeline (Fastify Hooks & Plugins)

Fastify's lifecycle hooks replace Express's middleware chain. Each stage maps to a
specific Fastify hook, giving precise control over execution order:

```
Incoming Request
      |
      v
[1] TLS Termination (handled by Cloud LB in prod / Node TLS locally)
      |
      v
[2] onRequest hook: User-Agent Parser
    - Validate PRD-TS43 format
    - Extract client_vendor, client_version, client_type, OS
    - Decorate request with parsed UA fields
      |
      v
[3] onRequest hook: Request Normalizer
    - GET: coerce query string params into EntitlementRequest shape
    - POST: Fastify's built-in JSON Schema validation handles this automatically
    - Decorate request with normalized EntitlementRequest
      |
      v
[4] onRequest hook: Version Check
    - Compare entitlement_version against supported versions
    - Reply 406 if unsupported (short-circuits remaining hooks)
      |
      v
[5] preHandler hook: Authentication
    - If token present → Redis lookup (fast-auth validation)
    - If eap_relay present → continue EAP-AKA exchange
    - If neither → start new EAP-AKA challenge (reply 401)
    - Decorate request with authenticated subscriber context
      |
      v
[6] Route Handler: Entitlement Service Router
    - Route by AppID to service handler plugin
    - Execute operation (if ODSA)
      |
      v
[7] preSerialization hook: Response Builder
    - Build XML or JSON based on accept_content_type
    - Include new token, VERS, and service configuration
      |
      v
[8] HTTP Response (200, 401, 302, 4xx, 5xx)
```

### 5.3 Runtime Input Validation (requestSchemas.ts)

TypeScript interfaces (`requestTypes.ts`) enforce correctness at compile time but do
nothing at runtime. A malformed payload from the internet can crash the parser, inject
unexpected values, or exploit downstream logic. Fastify solves this: every route gets a
JSON Schema that Ajv compiles into a validator at startup. Payloads that don't conform
are rejected with a 400 before handler code runs.

**Two layers, one source of truth:**

| Layer | File | When | Purpose |
|-------|------|------|---------|
| TypeBox schema | `requestSchemas.ts` | Startup (compiled to Ajv) + Runtime (validates every request) | Reject bad payloads, enforce types/formats/lengths |
| Inferred types | `requestTypes.ts` | Compile time | IDE autocomplete, type-safe handler code — `Static<typeof schema>` |

TypeBox defines schemas in TypeScript syntax that compile to standard JSON Schema.
Fastify feeds that JSON Schema to Ajv at startup. TypeScript types are inferred from
the same schema via `Static<>`, so validation rules and types can never drift apart.
There is no separate type definition to maintain.

**Why TypeBox over Zod:** Zod is a runtime validator — it interprets schemas on every
request. To use Zod with Fastify, you either bypass Fastify's compiled Ajv pipeline
(losing the performance advantage we chose Fastify for) or use an adapter that converts
Zod to JSON Schema under the hood (adding a translation layer for no benefit). TypeBox
outputs native JSON Schema directly, so Fastify compiles it into Ajv validators at
startup with zero translation overhead. TypeBox also integrates with Fastify's response
serialization via `fast-json-stringify`, which Zod cannot do.

**Schema definition example:**

```typescript
// src/protocol/requestSchemas.ts
import { Type, Static } from '@sinclair/typebox'

const AppId = Type.Union([
  Type.Literal('ap2003'), Type.Literal('ap2004'), Type.Literal('ap2005'),
  Type.Literal('ap2006'), Type.Literal('ap2009'), Type.Literal('ap2010'),
  Type.Literal('ap2011'), Type.Literal('ap2012'), Type.Literal('ap2013'),
  Type.Literal('ap2014'), Type.Literal('ap2015'), Type.Literal('ap2016'),
])

const OdsaOperation = Type.Union([
  Type.Literal('CheckEligibility'),   Type.Literal('ManageSubscription'),
  Type.Literal('ManageService'),      Type.Literal('AcquireConfiguration'),
  Type.Literal('AcquirePlan'),        Type.Literal('AcquireTemporaryToken'),
  Type.Literal('GetPhoneNumber'),     Type.Literal('VerifyPhoneNumber'),
  Type.Literal('GetSubscriberInfo'),
])

export const EntitlementRequestBody = Type.Object({
  // Required fields
  app:                  AppId,
  terminal_id:          Type.String({ pattern: '^[0-9]{15}$' }),          // IMEI: exactly 15 digits
  entitlement_version:  Type.Union([Type.Literal('2'), Type.Literal('4')]),

  // Device identification
  terminal_vendor:      Type.Optional(Type.String({ maxLength: 128 })),
  terminal_model:       Type.Optional(Type.String({ maxLength: 128 })),
  terminal_sw_version:  Type.Optional(Type.String({ maxLength: 128 })),

  // Authentication
  token:                Type.Optional(Type.String({ maxLength: 512 })),
  eap_relay:            Type.Optional(Type.String({ maxLength: 4096 })),  // base64 EAP packet

  // Subscriber identifiers
  IMSI:                 Type.Optional(Type.String({ pattern: '^[0-9]{5,15}$' })),
  MSISDN:               Type.Optional(Type.String({ pattern: '^\\+?[0-9]{5,15}$' })),
  EID:                  Type.Optional(Type.String({ pattern: '^[0-9]{32}$' })),
  ICCID:                Type.Optional(Type.String({ pattern: '^[0-9]{19,20}$' })),

  // ODSA-specific
  operation:            Type.Optional(OdsaOperation),
  operation_type:       Type.Optional(Type.Integer({ minimum: 0, maximum: 99 })),

  // Response format
  accept_content_type:  Type.Optional(Type.Union([Type.Literal('xml'), Type.Literal('json')])),
}, { additionalProperties: false })

// TypeScript type inferred from schema — no manual interface to maintain
export type EntitlementRequest = Static<typeof EntitlementRequestBody>
```

**What this protects against:**

| Threat | Schema defense |
|--------|----------------|
| SQL injection in IMSI/MSISDN | `pattern` restricts to digits only |
| Oversized payloads crashing parser | `maxLength` on every string field |
| Unexpected fields smuggling data | `additionalProperties: false` |
| Invalid AppID routing to undefined handler | `enum` whitelist |
| Type confusion (string where number expected) | `type: 'integer'` with bounds |
| Prototype pollution via `__proto__` | `additionalProperties: false` + Fastify's default prototype removal |

**Fastify integration:**

```typescript
// src/server/routes/entitlement.ts
import { TypeBoxTypeProvider } from '@fastify/type-provider-typebox'
import { EntitlementRequestBody } from '../protocol/requestSchemas.js'

app.withTypeProvider<TypeBoxTypeProvider>().post('/entitlement', {
  schema: { body: EntitlementRequestBody },
}, async (request, reply) => {
  // request.body is fully typed AND runtime-validated — no manual cast needed
  const { app, terminal_id, operation } = request.body;
  // TypeScript knows app is 'ap2003' | 'ap2004' | ... (union of literals)
  // ...
});
```

Invalid requests never reach the handler. Fastify returns a structured 400:

```json
{
  "statusCode": 400,
  "error": "Bad Request",
  "message": "body/terminal_id must match pattern \"^[0-9]{15}$\""
}
```

---

## 6. Entitlement Response Format

### 6.1 XML Format (WAP-Provisioning)

```xml
<?xml version="1.0"?>
<wap-provisioningdoc version="1.1">
  <characteristic type="VERS">
    <parm name="version" value="1"/>
    <parm name="validity" value="172800"/>
  </characteristic>
  <characteristic type="TOKEN">
    <parm name="token" value="kj34n5kj45n3k5..."/>
  </characteristic>
  <characteristic type="APPLICATION">
    <parm name="AppID" value="ap2004"/>
    <parm name="EntitlementStatus" value="1"/>
    <parm name="AddrStatus" value="1"/>
    <parm name="TC_Status" value="2"/>
    <parm name="ProvStatus" value="1"/>
    <characteristic type="ADDR">
      <parm name="AddrType" value="1"/>
      <parm name="Addr" value="pcscf.operator.com"/>
    </characteristic>
  </characteristic>
</wap-provisioningdoc>
```

### 6.2 JSON Format

```json
{
  "Vers": {
    "version": "1",
    "validity": "172800"
  },
  "Token": {
    "token": "kj34n5kj45n3k5..."
  },
  "ap2004": {
    "EntitlementStatus": "1",
    "AddrStatus": "1",
    "TC_Status": "2",
    "ProvStatus": "1",
    "Addr": {
      "1": {
        "AddrType": "1",
        "Addr": "pcscf.operator.com"
      }
    }
  }
}
```

### 6.3 Response Builder Design

The `responseBuilder.ts` module takes a typed service response object and serializes to the requested format:

```typescript
interface ServiceEntitlementResponse {
  version: string;
  validity: number;           // TTL in seconds
  token: string;
  applications: ApplicationConfig[];
}

interface ApplicationConfig {
  appId: string;              // "ap2003", "ap2004", etc.
  entitlementStatus: EntitlementStatus;
  params: Record<string, string | number | nested>;
}
```

Both `xmlBuilder` and `jsonBuilder` consume this same structure and emit the correct wire format.

---

## 7. Database Schema

### 7.1 Core Tables

```sql
-- Subscriber identity (maps to SIM/eSIM)
CREATE TABLE subscribers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    imsi            VARCHAR(15) UNIQUE NOT NULL,
    msisdn          VARCHAR(15),
    iccid           VARCHAR(20),
    eid             VARCHAR(32),
    ki_encrypted    BYTEA NOT NULL,     -- Ki encrypted with DEK (never stored plaintext)
    op_encrypted    BYTEA NOT NULL,     -- OP encrypted with DEK (never stored plaintext)
    ki_dek_wrapped  BYTEA NOT NULL,     -- DEK wrapped (encrypted) by Cloud KMS KEK
    sqn             BIGINT DEFAULT 0,   -- Sequence number for AKA replay protection
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Device registration
CREATE TABLE devices (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscriber_id   UUID REFERENCES subscribers(id),
    imei            VARCHAR(15) NOT NULL,
    vendor          VARCHAR(128),
    model           VARCHAR(128),
    sw_version      VARCHAR(128),
    device_type     VARCHAR(64),        -- "Smartphone", "Smartwatch", "Tablet"
    eid             VARCHAR(32),
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Per-service entitlement records
CREATE TABLE entitlements (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscriber_id   UUID NOT NULL REFERENCES subscribers(id),
    app_id          VARCHAR(10) NOT NULL,    -- "ap2003", "ap2004", etc.
    status          INTEGER NOT NULL DEFAULT 0,  -- 0=DISABLED, 1=ENABLED, 2=INCOMPATIBLE, 3=PROVISIONING
    prov_status     INTEGER DEFAULT 0,
    tc_status       INTEGER DEFAULT 0,
    config_data     JSONB,                   -- Service-specific configuration (addresses, PLMNs, etc.)
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(subscriber_id, app_id)
);

-- EAP-AKA sessions: stored in Redis, NOT Postgres.
-- Sessions are ephemeral (seconds), high-throughput, and losing one just
-- means the device re-authenticates. Postgres write I/O would be the
-- bottleneck during mass re-authentication events (network failover, etc.).
--
-- Redis key schema:
--   eap_session:{session_id} → Hash {
--     subscriber_id, imei, state,
--     rand, xres, ck, ik, autn,
--     mk, k_aut, k_encr
--   }
--   TTL: 90 seconds (allows for slow mobile networks, SQN re-sync,
--     SIM processing time, and brief app backgrounding)

-- Audit trail
CREATE TABLE audit_log (
    id              BIGSERIAL PRIMARY KEY,
    timestamp       TIMESTAMPTZ DEFAULT NOW(),
    subscriber_id   UUID,
    app_id          VARCHAR(10),
    operation       VARCHAR(64),
    request_summary JSONB,
    response_code   INTEGER,
    client_ip       INET,
    user_agent      TEXT
);
```

---

## 8. Ki Security: Mock HSS as a Separate Service

Ki (the 128-bit subscriber secret key) is the root of trust for EAP-AKA authentication.
If Ki is compromised, an attacker can impersonate any subscriber, derive session keys,
and intercept communications. Protecting Ki is the single most critical security
requirement in this system.

### 8.1 Architecture Principle: The ECS Never Touches Ki

In production, the HSS/AuC holds Ki in hardware and exposes only pre-computed AKA
vectors (RAND, AUTN, XRES, CK, IK) over Diameter. Ki never crosses the HSS boundary.
The ECS is an internet-facing HTTP server — it has no business holding or decrypting
subscriber secret keys.

**Our POC must follow the same principle.** Even though we don't have a real HSS, we
do NOT embed Ki handling inside the ECS. Instead, we run the mock HSS as a **separate
internal service** that the ECS calls for AKA vectors, exactly as it would call a
real HSS in production.

### 8.2 Two-Service Architecture

```
Internet (untrusted)
    |
    v
┌─────────────────────────────────────────┐
│  Cloud Run: entitlement-server (ECS)    │
│  - Public ingress (via Load Balancer)   │
│  - Handles HTTP, EAP-AKA relay, tokens  │
│  - NO access to Ki, NO KMS permissions  │
│  - Calls mock-hss for AKA vectors       │
└──────────────┬──────────────────────────┘
               │ (VPC internal only)
               v
┌─────────────────────────────────────────┐
│  Cloud Run: mock-hss                    │
│  - Internal-only ingress (no internet)  │
│  - Holds encrypted Ki (envelope enc.)   │
│  - Has KMS IAM role to unwrap DEKs      │
│  - Runs MILENAGE, returns vectors only  │
└──────────────┬──────────────────────────┘
               │
               v
         Cloud KMS (KEK)
```

**Why this matters:**
- If the ECS is compromised (it's internet-facing — this is the most likely breach
  point), the attacker gets tokens and entitlement data but **never Ki**. The ECS
  has no KMS permissions and no access to encrypted Ki material.
- The mock HSS has no public endpoint. It's unreachable from the internet.
- KMS IAM role is scoped exclusively to the mock HSS service account.
- Swapping the mock HSS for a real Diameter-connected HSS later means changing one
  URL in the ECS config — no rearchitecting.

### 8.3 Mock HSS API

The mock HSS exposes a single internal endpoint:

```
POST /vectors
Content-Type: application/json

Request:
{
  "imsi": "001010000000001",
  "sqn":  42
}

Response:
{
  "rand":  "<base64 128-bit>",
  "autn":  "<base64 128-bit>",
  "xres":  "<base64 variable>",
  "ck":    "<base64 128-bit>",
  "ik":    "<base64 128-bit>"
}
```

The response contains derived vectors only. Ki never appears in any request or response.

### 8.4 Envelope Encryption (Inside Mock HSS)

The mock HSS uses **envelope encryption** with Cloud KMS to protect Ki at rest:

```
Cloud KMS
  └── KEK (Key Encryption Key) — never leaves KMS hardware
        |
        └── Wraps → DEK (Data Encryption Key) — unique per subscriber
                      |
                      └── Encrypts → Ki + OP (plaintext never stored)
```

**Vector generation flow (inside mock HSS):**

```
1. Receive request: { imsi, sqn }
2. Read ki_encrypted, op_encrypted, ki_dek_wrapped from Postgres
3. Call Cloud KMS to unwrap the DEK: DEK = KMS_Decrypt(KEK, ki_dek_wrapped)
4. Decrypt Ki: Ki = AES-256-GCM-Decrypt(DEK, ki_encrypted)
5. Decrypt OP: OP = AES-256-GCM-Decrypt(DEK, op_encrypted)
6. Run MILENAGE(Ki, OP, RAND, SQN, AMF) → XRES, CK, IK, AUTN
7. Immediately zero out Ki, OP, and DEK from memory (overwrite Buffer)
8. Return only the derived vectors
```

### 8.5 Per-Subscriber DEKs

A single DEK for all subscribers would mean one compromised DEK exposes every Ki in the
database. Per-subscriber DEKs limit the blast radius: compromising one wrapped DEK
(which still requires KMS access) exposes only one subscriber's Ki.

### 8.6 What This Protects Against

| Threat | Mitigation |
|--------|------------|
| ECS compromised (internet-facing) | ECS has no Ki, no KMS access — nothing to steal |
| Database dump / SQL injection | Ki is ciphertext; useless without DEK |
| Stolen database backup | DEK is wrapped; useless without Cloud KMS access |
| Compromised mock HSS | Requires separate breach of internal-only service; KMS access can be revoked |
| Memory dump of mock HSS | Ki is zeroed immediately after use; exposure window is milliseconds |
| Insider with DB access | Cannot decrypt without KMS IAM role (scoped to mock HSS only) |
| KMS key compromise alone | Cannot decrypt without the per-subscriber wrapped DEK from DB |

The ECS, the database, AND KMS must all be compromised to extract Ki — and only the
mock HSS bridges the last two, and it has no internet exposure.

### 8.7 Local Development

For local dev (Docker Compose), the mock HSS runs as a separate container with a
**local KEK from an environment variable** simulating envelope encryption:

```typescript
// mock-hss/src/kms.ts
interface KeyManager {
  wrapDek(dek: Buffer): Promise<Buffer>;
  unwrapDek(wrappedDek: Buffer): Promise<Buffer>;
}

// GCP: Cloud KMS
class CloudKmsKeyManager implements KeyManager { ... }

// Local dev: symmetric key from env var (same encrypt/decrypt interface)
class LocalKeyManager implements KeyManager { ... }
```

The ECS always calls `${HSS_URL}/vectors` — it doesn't know or care whether the HSS
is mock or real. Same interface contract, same network boundary. The `eapAkaVectors.ts`
client in the ECS is a thin HTTP caller:

```typescript
// src/auth/eapAkaVectors.ts
interface AkaVectors {
  rand: Buffer;   // 128-bit
  autn: Buffer;   // 128-bit
  xres: Buffer;   // 4-16 bytes
  ck:   Buffer;   // 128-bit
  ik:   Buffer;   // 128-bit
}

async function getAkaVectors(imsi: string, sqn: number): Promise<AkaVectors> {
  // HSS_URL from config — mock-hss in POC, real HSS gateway in production
  const res = await fetch(`${config.HSS_URL}/vectors`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ imsi, sqn }),
  });
  // ... parse response into AkaVectors
}
```

This function is the **only integration point**. Migrating to a real HSS means:
1. Set `HSS_URL` to the Diameter-to-HTTP gateway
2. Ensure the gateway returns the same `{ rand, autn, xres, ck, ik }` JSON contract
3. No ECS code changes

### 8.8 Logging and Audit

- **Never log Ki, OP, DEK, or wrapped DEK** — not at any log level, not in error messages
- **Log KMS access** — Cloud KMS provides built-in audit logs for every encrypt/decrypt call
- **Redact in error paths** — if MILENAGE fails, log the IMSI and error type, never the key material
- **Log all /vectors requests** in the mock HSS — IMSI requested, timestamp, caller IP (audit trail)

---

## 9. Service Handlers (Per AppID)

### 8.1 VoWiFi Entitlement (ap2004) — Priority 1

The simplest and most common entitlement check. Ideal first implementation target.

**Request:** Device asks "am I allowed to use VoWiFi?"
**Response:** EntitlementStatus + provisioning addresses (P-CSCF, etc.)

**Key fields returned:**
- `EntitlementStatus`: 0=DISABLED, 1=ENABLED, 2=INCOMPATIBLE, 3=PROVISIONING
- `AddrStatus`: Whether address list is valid
- `TC_Status`: Terms & Conditions status
- `ProvStatus`: Provisioning status on IMS network
- `Addr[]`: P-CSCF server addresses for the device to use
- `ServiceFlow_URL`: URL for T&C acceptance webview (if TC_Status requires it)

**Logic:**
1. Look up subscriber entitlement for ap2004
2. If ENABLED and provisioned → return addresses
3. If DISABLED with TC_Status=AVAILABLE → return ServiceFlow_URL for T&C
4. If INCOMPATIBLE → return MessageForIncompatible

### 8.2 Voice-over-Cellular (ap2003) — Priority 1

Nearly identical to VoWiFi but for VoLTE (4G) and VoNR (5G).

**Additional fields:**
- `VoLTE_Entitled`: VoLTE-specific status
- `VoNR_Entitled`: VoNR-specific status
- Home vs. roaming entitlement variants

### 8.3 SMSoIP (ap2005) — Priority 1

Minimal: just EntitlementStatus + address configuration. Same pattern as VoWiFi.

### 8.4 ODSA Companion Device (ap2006) — Priority 2

Complex multi-step flows for eSIM companion device management.

**Operations:**
- `CheckEligibility` → Is user eligible for companion device?
- `ManageSubscription` → Activate, transfer, or deactivate eSIM subscription
- `ManageService` → Enable/disable specific services on companion
- `AcquireConfiguration` → Get current subscription config
- `AcquireTemporaryToken` → Generate scoped token for device transfer

**Key SubscriptionResult codes driving flow:**
| Code | Meaning | Action |
|------|---------|--------|
| 1 | CONTINUE_TO_WS | Open SP web portal |
| 2 | DOWNLOAD_PROFILE | Return SM-DP+ activation code |
| 3 | DONE | Operation complete |
| 4 | DELAYED_DOWNLOAD | Profile being prepared; poll or wait for push |
| 6 | DELETE_PROFILE_IN_USE | Device must delete old profile first |
| 7 | REQUIRES_USER_INPUT | Show MSG dialog to user |

### 8.5 ODSA Primary Device (ap2009) — Priority 2

Same operations as companion but for the device's own subscription. Additional considerations:
- Plan acquisition (`AcquirePlan`) to list available subscription plans
- More complex transfer flows (OTP, OAuth, EAP-AKA, temporary token variants)

### 8.6 Data Plan Information (ap2010) — Priority 3

Returns current data plan details, usage, and boost eligibility.

**Key structures:**
- `DataPlanInfo`: AccessType, DataPlanType (metered/unmetered)
- `DataUsageInfo`: Allowance, used bytes, billing cycle end
- `DataBoostInfo`: Boost type eligibility, QoS parameters (PDB, jitter, rates)

### 8.7 Server-Initiated ODSA (ap2011) — Priority 3

Enterprise/MDM flow. Uses server-to-server OAuth 2.0 with JWT client assertion.

**Three-tier token model:**
1. OAuth Access Token (from authorization server)
2. Auth Token (from ECS, scoped to enterprise_id)
3. Per-device operations using the auth token

### 8.8 Direct Carrier Billing (ap2012) — Priority 3

Entitlement check for mobile payment capability.

**State matrix (EntitlementStatus x TC_Status):**
- INCOMPATIBLE + any → show incompatible message
- DISABLED + NOT_AVAILABLE → open websheet for T&C
- DISABLED + AVAILABLE → service being provisioned
- ENABLED + AVAILABLE/NOT_REQUIRED → can purchase

### 8.9 Private User Identity (ap2013) — Priority 4

WiFi authentication using pseudonym or encrypted IMSI to prevent tracking.

**Two types:**
- Type 1 (PSEUDONYM): `MK = SHA1(Pseudonym | IK | CK)`
- Type 2 (OTHER/encrypted): `MK = SHA1(IMSI | IK | CK)`

### 8.10 Device and User Info (ap2014) — Priority 4

Returns MSISDN and subscriber info. Supports GetPhoneNumber, VerifyPhoneNumber, GetSubscriberInfo operations.

### 8.11 App Authentication (ap2015) — Priority 4

OperatorToken and AppToken flows for third-party app integration.

### 8.12 SatMode (ap2016) — Priority 4

Satellite connectivity entitlement. Returns PLMN allow/barred lists with service constraints.

---

## 9. HTTP Response Codes

The server must return these specific codes per the spec:

| Code | Meaning | When |
|------|---------|------|
| 200 | OK | Successful entitlement response |
| 302 | Found | Redirect to OIDC provider |
| 400 | Bad Request | Missing/invalid parameters |
| 401 | Unauthorized | EAP-AKA challenge (carries EAP-Request in body) |
| 403 | Forbidden | Authentication failed / invalid identity |
| 405 | Method Not Allowed | POST not supported (if GET-only) |
| 406 | Not Acceptable | Unsupported entitlement_version |
| 500 | Internal Server Error | Server failure |
| 501 | Not Implemented | Requested AppID/operation not supported |
| 503 | Service Unavailable | Temporary overload (include Retry-After header) |
| 511 | Network Auth Required | Must authenticate first |

---

## 10. User-Agent Header Format

TS.43 defines a specific User-Agent format that the server must parse:

```
PRD-TS43/<client_version> (<vendor>; <terminal_model>; <client_type>; <OS>) <extra>
```

Example:
```
PRD-TS43/2.0 (Samsung; SM-G998B; Android-Client; Android 13) OMAClient
```

The middleware extracts and validates these fields. Requests with missing or malformed User-Agent headers should still be processed but flagged in audit logs.

---

## 11. Notification System

### 11.1 SMS Notification (Port 8095 / 0x1F9F)

The spec defines SMS-based push notifications using application port addressing. For the POC, we implement the server-side message construction but not actual SMS delivery.

**SMS payload format:** Binary SMS to port 8095 with a flag indicating which service changed.

### 11.2 Cloud Push Notifications

Server sends push via FCM (Android), APNS (iOS), or WNS (Windows) to tell device to re-check entitlements.

**For POC:** Implement the notification trigger logic and message format. Actual push delivery can be mocked.

---

## 12. Observability: Logging, Tracing, and Monitoring

Observability is critical for monitoring SLIs, debugging production issues, and
alerting on degraded service before users notice. We use the three pillars of
GCP's operations suite: Cloud Logging (structured logs), Cloud Trace (distributed
tracing), and Cloud Monitoring (metrics and alerting).

### 12.1 Cloud Logging (Structured Logs)

Pino is already in the tech stack. Its JSON output is auto-ingested by Cloud
Logging from Cloud Run's stdout. We add structure to make logs queryable and
correlated.

**Log format:**

```typescript
// Every log line includes these fields for Cloud Logging correlation
{
  "severity": "INFO",                          // Cloud Logging severity level
  "message": "EAP-AKA challenge sent",
  "logging.googleapis.com/trace": "projects/PROJECT/traces/TRACE_ID",
  "logging.googleapis.com/spanId": "SPAN_ID",
  "logging.googleapis.com/operation": {
    "id": "eap-aka-session-abc123",
    "producer": "entitlement-server"
  },
  // Structured fields for filtering
  "imsi": "001010000000001",
  "app_id": "ap2004",
  "operation": "CheckEligibility",
  "session_id": "abc123",
  "auth_method": "eap-aka",
  "response_code": 200,
  "latency_ms": 42
}
```

**Key logging rules:**
- Every request logs: app_id, operation, terminal_id, response code, latency
- Every EAP-AKA step logs: session_id, state transition, auth_method
- **Never log:** Ki, OP, DEK, wrapped DEK, token values, eap_relay content, IMSI
  in combination with location data
- **Redact tokens in logs:** log only the last 8 chars (`token: "...kj45n3k5"`)
- Attach trace ID and span ID to every log line so logs correlate with traces

**Pino setup with Cloud Logging integration:**

```typescript
// src/config/logger.ts
import pino from 'pino';

export const logger = pino({
  formatters: {
    level(label: string) {
      const severityMap: Record<string, string> = {
        trace: 'DEBUG', debug: 'DEBUG', info: 'INFO',
        warn: 'WARNING', error: 'ERROR', fatal: 'CRITICAL',
      };
      return { severity: severityMap[label] || 'DEFAULT' };
    },
  },
  mixin() {
    // Attach trace context from the current request (set in onRequest hook)
    const trace = getActiveTraceContext();
    if (trace) {
      return {
        'logging.googleapis.com/trace': trace.traceId,
        'logging.googleapis.com/spanId': trace.spanId,
      };
    }
    return {};
  },
});
```

### 12.2 Cloud Trace (Distributed Tracing)

An EAP-AKA request touches multiple services: ECS → Redis → mock HSS → Postgres
→ KMS. Without tracing, debugging latency issues requires correlating logs across
services by timestamp — unreliable and slow.

**Integration:** Use `@google-cloud/opentelemetry-cloud-trace-exporter` with the
OpenTelemetry SDK. Cloud Run auto-injects trace context headers
(`X-Cloud-Trace-Context`), so traces propagate across the ECS → mock HSS boundary
automatically.

```typescript
// src/config/tracing.ts
import { NodeSDK } from '@opentelemetry/sdk-node';
import { TraceExporter } from '@google-cloud/opentelemetry-cloud-trace-exporter';
import { HttpInstrumentation } from '@opentelemetry/instrumentation-http';
import { PgInstrumentation } from '@opentelemetry/instrumentation-pg';
import { IORedisInstrumentation } from '@opentelemetry/instrumentation-ioredis';

const sdk = new NodeSDK({
  traceExporter: new TraceExporter(),
  instrumentations: [
    new HttpInstrumentation(),      // Traces all HTTP calls (ECS ↔ mock HSS)
    new PgInstrumentation(),        // Traces all Postgres queries
    new IORedisInstrumentation(),   // Traces all Redis commands
  ],
});

sdk.start();
```

**What gets traced automatically:**

| Span | Service | What it shows |
|------|---------|---------------|
| `POST /entitlement` | ECS | Full request lifecycle, total latency |
| `Redis GET token:{value}` | ECS | Token cache lookup time, hit/miss |
| `Redis HGETALL eap_session:{id}` | ECS | Session lookup time |
| `POST /vectors` | ECS → mock HSS | HSS call latency, network time |
| `SELECT FROM subscribers` | mock HSS | Ki lookup query time |
| `KMS Decrypt` | mock HSS | DEK unwrap latency |

**Custom spans for business logic:**

```typescript
// src/auth/eapAka.ts
import { trace } from '@opentelemetry/api';

const tracer = trace.getTracer('entitlement-server');

async function handleEapResponse(request: EntitlementRequest) {
  return tracer.startActiveSpan('eap-aka.verify-response', async (span) => {
    span.setAttribute('session_id', sessionId);
    span.setAttribute('state_transition', 'CHALLENGE_SENT→SUCCESS');

    // ... verify RES, derive keys, issue token

    span.setAttribute('auth.success', true);
    span.end();
  });
}
```

**Trace-log correlation:** The trace ID and span ID are attached to every Pino log
line (see 12.1), so clicking a trace in Cloud Trace shows the correlated logs
inline — no manual timestamp matching needed.

### 12.3 Cloud Monitoring (Metrics and Dashboards)

Cloud Run exports basic metrics automatically (request count, latency, instance
count, CPU/memory). We add **custom application metrics** for ECS-specific SLIs
using the OpenTelemetry Metrics SDK exported to Cloud Monitoring.

**Custom metrics:**

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `ecs/eap_aka/attempts` | Counter | `result`: success, failure, sync_failure | Track auth success rate |
| `ecs/eap_aka/latency` | Histogram | `phase`: challenge, verify, total | Identify bottleneck in auth flow |
| `ecs/token/cache_hit_ratio` | Gauge | `token_type`: auth, fast_auth | Monitor write-through cache effectiveness |
| `ecs/token/validations` | Counter | `source`: redis_hit, postgres_fallback, expired, invalid | Token validation path breakdown |
| `ecs/entitlement/requests` | Counter | `app_id`, `operation`, `status_code` | Per-service request volume and error rates |
| `ecs/entitlement/latency` | Histogram | `app_id`, `operation` | Per-service latency distribution |
| `ecs/hss/call_latency` | Histogram | — | Mock HSS round-trip time (detect HSS degradation) |
| `ecs/hss/errors` | Counter | `error_type` | HSS call failures |
| `ecs/db/pool_utilization` | Gauge | — | Active connections / DB_POOL_SIZE (detect pool exhaustion) |
| `ecs/db/query_latency` | Histogram | `query_type`: token_read, token_write, entitlement_lookup | Per-query latency |
| `ecs/idempotency/cache_hits` | Counter | — | Retry replays (detect network issues causing retries) |

**Metrics implementation:**

```typescript
// src/config/metrics.ts
import { MeterProvider } from '@opentelemetry/sdk-metrics';
import { MetricExporter } from '@google-cloud/opentelemetry-cloud-monitoring-exporter';

const meterProvider = new MeterProvider({
  readers: [new PeriodicExportingMetricReader({
    exporter: new MetricExporter(),
    exportIntervalMillis: 60000,   // Export every 60s
  })],
});

const meter = meterProvider.getMeter('entitlement-server');

// EAP-AKA metrics
export const eapAkaAttempts = meter.createCounter('ecs.eap_aka.attempts');
export const eapAkaLatency = meter.createHistogram('ecs.eap_aka.latency', {
  unit: 'ms',
  description: 'EAP-AKA authentication latency by phase',
});

// Token metrics
export const tokenValidations = meter.createCounter('ecs.token.validations');

// Entitlement metrics
export const entitlementRequests = meter.createCounter('ecs.entitlement.requests');
export const entitlementLatency = meter.createHistogram('ecs.entitlement.latency', {
  unit: 'ms',
});

// HSS metrics
export const hssCallLatency = meter.createHistogram('ecs.hss.call_latency', {
  unit: 'ms',
});

// DB pool metrics
export const dbPoolUtilization = meter.createObservableGauge('ecs.db.pool_utilization');
dbPoolUtilization.addCallback((result) => {
  result.observe(pool.totalCount / pool.options.max!);  // 0.0 to 1.0
});
```

### 12.4 SLIs and Alerting Policies

Define SLIs based on the metrics above, with Cloud Monitoring alerting policies
that fire before users are impacted.

**Service Level Indicators:**

| SLI | Definition | Target |
|-----|-----------|--------|
| Availability | `ecs/entitlement/requests` with `status_code < 500` / total | 99.9% |
| Auth latency (p99) | `ecs/eap_aka/latency` with `phase=total`, 99th percentile | < 2s |
| Entitlement latency (p99) | `ecs/entitlement/latency`, 99th percentile | < 500ms |
| Token cache hit rate | `ecs/token/validations` with `source=redis_hit` / total | > 95% |
| Auth success rate | `ecs/eap_aka/attempts` with `result=success` / total | > 99% |

**Alerting policies (Cloud Monitoring):**

| Alert | Condition | Severity | Action |
|-------|-----------|----------|--------|
| High error rate | 5xx rate > 1% over 5 min | Critical | Page on-call |
| Auth latency spike | EAP-AKA p99 > 5s over 5 min | Warning | Notify channel |
| HSS degradation | `ecs/hss/call_latency` p99 > 3s over 5 min | Warning | Notify channel |
| DB pool saturation | `ecs/db/pool_utilization` > 0.8 over 5 min | Warning | Notify channel |
| Token cache miss spike | cache hit ratio < 80% over 10 min | Warning | Investigate Redis |
| EAP-AKA failure spike | auth success rate < 95% over 5 min | Critical | Page on-call |
| Cloud Run instance ceiling | instance count > 80 (of 100 max) | Warning | Consider scaling maxInstances |

**Dashboard layout (Cloud Monitoring):**

```
┌─────────────────────────────────────────────────────────┐
│  ECS Overview Dashboard                                 │
├──────────────────────┬──────────────────────────────────┤
│  Request Rate        │  Error Rate (4xx / 5xx)          │
│  (by app_id)         │  (by status_code)                │
├──────────────────────┼──────────────────────────────────┤
│  EAP-AKA Latency     │  EAP-AKA Success/Failure Rate    │
│  (p50, p95, p99)     │  (by result)                     │
├──────────────────────┼──────────────────────────────────┤
│  Token Cache Hit %   │  HSS Call Latency                │
│                      │  (p50, p95, p99)                 │
├──────────────────────┼──────────────────────────────────┤
│  DB Pool Utilization │  Cloud Run Instances             │
│  (active / max)      │  (current vs. min/max)           │
├──────────────────────┼──────────────────────────────────┤
│  Entitlement Latency │  Idempotency Cache Hits          │
│  (by app_id, p99)    │  (retry replay rate)             │
└──────────────────────┴──────────────────────────────────┘
```

### 12.5 Technology Additions

| Component | Package | Purpose |
|-----------|---------|---------|
| Tracing SDK | `@opentelemetry/sdk-node` | OpenTelemetry base SDK |
| Trace exporter | `@google-cloud/opentelemetry-cloud-trace-exporter` | Export spans to Cloud Trace |
| Metrics exporter | `@google-cloud/opentelemetry-cloud-monitoring-exporter` | Export metrics to Cloud Monitoring |
| HTTP instrumentation | `@opentelemetry/instrumentation-http` | Auto-trace HTTP calls |
| Postgres instrumentation | `@opentelemetry/instrumentation-pg` | Auto-trace DB queries |
| Redis instrumentation | `@opentelemetry/instrumentation-ioredis` | Auto-trace Redis commands |

These are initialized once at startup (`src/config/tracing.ts` and
`src/config/metrics.ts`) before Fastify starts. The auto-instrumentation libraries
patch `http`, `pg`, and `ioredis` modules transparently — no changes to application
code.

---

## 13. Implementation Phases

### Phase 1: Foundation
1. Project scaffolding (package.json, tsconfig, Docker Compose)
2. Fastify app with hooks and plugin structure
3. OpenTelemetry tracing + metrics SDK initialization (before Fastify starts)
4. Pino logger with Cloud Logging severity mapping and trace correlation
5. Request parser (GET + POST normalization)
6. User-Agent parser
7. Version negotiation
8. PostgreSQL schema + Drizzle setup
9. Basic health check / readiness endpoints

### Phase 2: Mock HSS Service
1. MILENAGE implementation in TypeScript (`mock-hss/src/milenage.ts`) — f1, f1*, f2345, f5* using Node.js `crypto` for AES-128-ECB
2. Unit tests against all official 3GPP TS 35.207 test vectors — must pass before proceeding
3. Envelope encryption (KMS/local KEK → per-subscriber DEK → encrypted Ki/OP)
4. Mock HSS Fastify server with `POST /vectors` endpoint
5. Docker container with internal-only network access

### Phase 3: EAP-AKA Authentication
1. EAP packet codec (encode/decode per RFC 4187)
2. EAP-AKA state machine (IDLE → CHALLENGE_SENT → SUCCESS/FAILURE)
3. `eapAkaVectors.ts` HTTP client calling `HSS_URL/vectors`
4. Key derivation (MK, K_encr, K_aut, MSK)
5. AT_MAC computation and verification
6. Auth session storage (Redis-only with 90s TTL)
7. Response idempotency cache (fingerprint → cached 200 OK, 90s TTL)
8. Integration test: full EAP-AKA handshake over HTTP (ECS → mock HSS → response)
9. Integration test: retry after dropped response returns cached response

### Phase 4: Token Management & Fast Auth
1. Auth token issuance on successful EAP-AKA
2. Token validation middleware
3. Fast-auth token flow (skip EAP-AKA on valid token)
4. Token expiry and rotation
5. Token persistence across server restarts

### Phase 5: Core Entitlement Services
1. Response builder (XML + JSON dual format)
2. VoWiFi (ap2004) handler
3. Voice-over-Cellular (ap2003) handler
4. SMSoIP (ap2005) handler
5. Seed data for test subscribers with entitlements

### Phase 6: ODSA Flows
1. ODSA operation router (CheckEligibility, ManageSubscription, etc.)
2. Companion device (ap2006) — activation, transfer flows
3. Primary device (ap2009) — activation, plan acquisition
4. Temporary token issuance and validation
5. SubscriptionResult handling (portal redirect, delayed download, user input)
6. Mock SM-DP+ for profile activation codes

### Phase 7: Extended Services
1. Data Plan Information (ap2010)
2. Server-initiated ODSA (ap2011) with JWT auth
3. Direct Carrier Billing (ap2012)
4. Remaining services (ap2013–ap2016) as stubs with correct response format

### Phase 8: GCP Infrastructure & CI/CD
1. Generate Drizzle migration files and create migration runner script
2. Implement CloudKmsKeyManager in mock-hss/src/kms.ts (alongside existing LocalKeyManager)
3. Update Dockerfiles and config defaults for Cloud Run (port 8080, pool size tuning)
4. Terraform modules: networking (VPC, subnets, VPC connector), database (Cloud SQL), redis (Memorystore), kms (keyring + crypto key), secrets (Secret Manager), artifact-registry, iam (service accounts), cloud-run (ECS + mock-hss), cloud-armor (WAF policy)
5. CI/CD pipeline: cloudbuild.yaml (build, test, push, migrate, deploy)
6. Cloud Build trigger (Terraform-managed, GitHub push to main)
7. Initial deployment: terraform apply, first image push, migrations, seed data
8. Smoke test: verify health endpoints, EAP-AKA challenge, Cloud Logging + Cloud Trace

### Phase 9: Observability
1. Custom application metrics (EAP-AKA counters, token cache hit ratio, HSS latency, DB pool utilization)
2. Custom spans for business logic (EAP-AKA phases, entitlement service routing)
3. Cloud Monitoring dashboard (request rate, error rate, latency, cache hits, pool utilization)
4. Alerting policies (error rate, auth latency, HSS degradation, pool saturation, instance ceiling)
5. Verify trace-log correlation (Cloud Trace → Cloud Logging linkage)

### Phase 10: Testing & Hardening
1. Unit tests for all codec/crypto functions
2. Integration tests for each entitlement flow
3. Load testing with simulated device traffic
4. Error handling audit (all spec HTTP codes)
5. Logging and audit trail completeness

---

## 14. Mock/Simulation Layer

Since this is a POC without a real HSS, SM-DP+, or BSS, we need mock implementations:

| Real Component | Mock Implementation |
|---------------|---------------------|
| HSS/AuC | **Separate `mock-hss` Cloud Run service** (internal-only); encrypted Ki in Postgres, MILENAGE in software, returns vectors only |
| 3GPP AAA | Built into ECS (no separate server for POC) |
| SM-DP+ | Returns canned activation codes and profile metadata |
| BSS/OSS | In-memory entitlement rules engine |
| Push (FCM/APNS) | Log-only; record notification payloads for inspection |

### Test Subscribers (Seed Data)

Test Ki values are defined in the seed script only (`src/mock/subscribers.ts`) and are
encrypted through the same envelope encryption path before insertion — even seed data
is never stored as plaintext Ki in the database.

```
Subscriber 1: "Alice"
  IMSI: 001010000000001
  MSISDN: +15551000001
  Ki: (defined in seed script, encrypted at insert time)
  VoWiFi: ENABLED, VoLTE: ENABLED, SMSoIP: ENABLED

Subscriber 2: "Bob"
  IMSI: 001010000000002
  MSISDN: +15551000002
  Ki: (defined in seed script, encrypted at insert time)
  VoWiFi: DISABLED (needs T&C), VoLTE: ENABLED

Subscriber 3: "Charlie"
  IMSI: 001010000000003
  MSISDN: +15551000003
  Ki: (defined in seed script, encrypted at insert time)
  All services: INCOMPATIBLE (test device)
```

---

## 15. Configuration

Environment variables:

```env
# Server
PORT=8443
HOST=0.0.0.0
TLS_CERT_PATH=./certs/server.crt
TLS_KEY_PATH=./certs/server.key

# Database
DATABASE_URL=postgresql://ecs:password@localhost:5432/entitlements
DB_POOL_SIZE=5                                   # Connections per Cloud Run instance (tune vs. maxInstances × pool ≤ max_connections)

# Redis
REDIS_URL=redis://localhost:6379

# Mock HSS
# HSS Vector Service (ECS is agnostic — same config whether mock or real)
HSS_URL=http://mock-hss:3001         # Local: Docker Compose service name
                                     # GCP POC: Cloud Run internal URL for mock-hss
                                     # Production: Diameter-to-HTTP gateway URL for real HSS

# Token settings
AUTH_TOKEN_TTL_SECONDS=86400        # 24 hours
FAST_AUTH_TOKEN_TTL_SECONDS=172800  # 48 hours
TEMP_TOKEN_TTL_SECONDS=3600        # 1 hour

# Ki Encryption (envelope encryption)
KMS_KEY_RING=entitlement-keys                              # Cloud KMS key ring name
KMS_KEY_NAME=ki-kek                                        # KEK name within the ring
KMS_LOCATION=us-central1                                   # KMS region
LOCAL_KEK_HEX=                                             # Local dev only: 256-bit hex key (ignored in prod)

# Protocol
SUPPORTED_VERSIONS=2,4
DEFAULT_CONFIG_VALIDITY=172800      # 48 hours

# Operator identity
OPERATOR_MCC=001
OPERATOR_MNC=01
OPERATOR_NAME=TestOperator
```

---

## 16. GCP Deployment Architecture

### 15.1 Infrastructure Mapping

| Component | Local Dev | GCP Production |
|-----------|-----------|----------------|
| ECS (entitlement server) | Docker container (Fastify) | **Cloud Run** (public ingress via LB) |
| Mock HSS | Docker container (Fastify) | **Cloud Run** (internal-only ingress, no internet) |
| PostgreSQL | Docker Compose container | **Cloud SQL for PostgreSQL** |
| Redis | Docker Compose container | **Memorystore for Redis** |
| DDoS / WAF | N/A (not needed locally) | **Cloud Armor** (DDoS mitigation + WAF security policy) |
| TLS termination | Self-signed certs in Node | **Global External Application Load Balancer** (managed certs) |
| Secrets (Ki, DB creds) | `.env` file | **Secret Manager** |
| Container registry | Local build | **Artifact Registry** |
| Logging | Pino → stdout | **Cloud Logging** (structured JSON auto-ingested) |
| Tracing | Console output (dev mode) | **Cloud Trace** (via OpenTelemetry exporter) |
| Metrics | Console output (dev mode) | **Cloud Monitoring** (via OpenTelemetry exporter) |
| Dashboards & Alerting | N/A | **Cloud Monitoring** dashboards + alerting policies |
| Push notifications (future) | Mock/log-only | **Firebase Cloud Messaging** |

### 15.2 Cloud Run Configuration

Cloud Run is the right fit here: the ECS is a stateless HTTP server (all state lives in Cloud SQL + Memorystore), and Cloud Run gives us auto-scaling, zero ops, and pay-per-request.

```yaml
# service.yaml (Cloud Run service definition)
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: entitlement-server
spec:
  template:
    metadata:
      annotations:
        run.googleapis.com/cloudsql-instances: PROJECT:REGION:INSTANCE
        run.googleapis.com/network-interfaces: '[{"network":"NETWORK","subnetwork":"SUBNET"}]'
        run.googleapis.com/minInstances: "1"      # Always-warm: eliminates cold start on first request
        run.googleapis.com/maxInstances: "100"    # Cap to protect Cloud SQL (100 × 2 pool = 200 connections)
    spec:
      containerConcurrency: 80
      containers:
        - image: REGION-docker.pkg.dev/PROJECT/ecs/entitlement-server:latest
          ports:
            - containerPort: 8080    # Plain HTTP — LB handles TLS
          env:
            - name: NODE_ENV
              value: production
            - name: DB_POOL_SIZE
              value: "5"         # Tune at deployment: maxInstances × DB_POOL_SIZE ≤ Cloud SQL max_connections
          resources:
            limits:
              memory: 512Mi
              cpu: "1"
          startupProbe:
            httpGet:
              path: /health          # Returns 200 only after Postgres + Redis connected
              port: 8080
            initialDelaySeconds: 0
            periodSeconds: 2
            failureThreshold: 5
            timeoutSeconds: 3
```

Key points:
- **Direct VPC Egress** (not VPC Connector) — Cloud Run instances are placed directly on a VPC subnet via `network-interfaces` annotation. No intermediate connector VMs, lower latency to Memorystore and Cloud SQL, no connector throughput bottleneck, and nothing extra to provision or pay for. VPC Connectors are the legacy approach.
- **Cloud SQL Auth Proxy** is built into Cloud Run via the `cloudsql-instances` annotation — no sidecar needed
- **containerPort 8080** — Cloud Run expects HTTP, not HTTPS; the managed load balancer terminates TLS
- **containerConcurrency: 80** — Fastify can handle many concurrent requests since most work is I/O (DB, Redis)

### 15.3 Cold Start Mitigation

Cold starts are a critical concern for this system. An EAP-AKA handshake requires
2+ HTTP round-trips, and a fully cold request chain stacks up:

```
ECS cold start (container boot + Node.js init + Fastify ready)   ~1-3s
  → Establish Redis connection                                    ~100ms
  → Call mock HSS (which may also be cold):
      Mock HSS cold start                                         ~1-3s
        → Establish Postgres connection                           ~200ms
        → KMS API call to unwrap DEK                              ~100ms
─────────────────────────────────────────────────────────────────
Worst case total:                                                 ~3-6s
```

Add mobile network latency on top, and the device's HTTP client timeout (often 30s)
is reachable. This must not happen.

**Solution: minimum instances (always-warm)**

Both Cloud Run services are configured with `minInstances: 1` so at least one
container is always warm and ready to handle requests with no cold start:

```yaml
# ECS service
metadata:
  annotations:
    run.googleapis.com/minInstances: "1"

# Mock HSS service
metadata:
  annotations:
    run.googleapis.com/minInstances: "1"
```

This means:
- The first request hits a warm container with live Redis/Postgres connections
- Auto-scaling still works — Cloud Run scales up from 1 (not 0) under load
- Cost is minimal: one idle instance per service, billed at Cloud Run idle rates
  (significantly reduced CPU billing when not handling requests)

**Application-level startup optimizations:**

Even with warm instances, new instances during scale-up events will cold start. We
minimize that impact:

1. **Eager connection pooling at startup** — establish Postgres and Redis connections
   during Fastify's `onReady` hook, not on first request:

```typescript
// src/server/app.ts
app.addHook('onReady', async () => {
  // Pre-warm connections so first request doesn't pay connection setup cost
  await db.execute(sql`SELECT 1`);      // Postgres connection pool warm-up
  await redis.ping();                    // Redis connection warm-up
});
```

2. **Mock HSS connection pre-warming** — same pattern in the mock HSS service for its
   Postgres and KMS client connections

3. **TypeBox schema compilation happens at startup** — Fastify compiles all Ajv
   validators during `app.listen()`, not on first request. This is already handled
   by Fastify's architecture.

4. **Lightweight container image** — use a minimal base image (`node:20-slim`) and
   avoid bundling dev dependencies to reduce container pull and boot time

**Startup probe configuration:**

Cloud Run's startup probe ensures traffic isn't routed to a container until it's
fully ready (connections established, schemas compiled):

```yaml
containers:
  - startupProbe:
      httpGet:
        path: /health
        port: 8080
      initialDelaySeconds: 0
      periodSeconds: 2
      failureThreshold: 5
      timeoutSeconds: 3
```

The `/health` endpoint returns 200 only after the `onReady` hook completes
(Postgres + Redis connected). Cloud Run won't route traffic until this passes.

### 15.4 Database Connection Management

The ECS is the internet-facing service that Cloud Run auto-scales under traffic.
It can go from 1 instance to 100+ in seconds during a mass re-authentication
event. Each instance opens a Postgres connection pool. Without controls, this
exhausts Cloud SQL's `max_connections` and cascading failures crash the system.

(The mock HSS is excluded from this analysis — it runs in a controlled test
environment with limited traffic, not exposed to unpredictable internet scale.)

**The math:**

```
Cloud Run ECS instances:      100 (burst scale-up)
Pool size per instance:       10  (typical default)
Total connections:            1,000
Cloud SQL max_connections:    100-500 (depends on instance tier)
                              → CONNECTION EXHAUSTION
```

**Defense in depth — three layers:**

**Layer 1: Small application-side pool (Drizzle/node-postgres)**

Each Cloud Run instance uses a minimal connection pool. The default of 10 is too
high for a serverless environment where many instances share one database.

```typescript
// src/db/schema.ts
import { Pool } from 'pg';
import { drizzle } from 'drizzle-orm/node-postgres';

const pool = new Pool({
  connectionString: config.DATABASE_URL,
  max: config.DB_POOL_SIZE,  // Default: 5 — configurable at deployment time
  idleTimeoutMillis: 30000,  // Release idle connections after 30s
  connectionTimeoutMillis: 5000,
});

export const db = drizzle(pool);
```

With default `DB_POOL_SIZE=5`: 100 ECS instances × 5 = 500 connections. In
practice, not all instances saturate their pool simultaneously, so this fits
within Cloud SQL's 500 default `max_connections` with normal headroom.

**Layer 2: Cloud Run `maxInstances` ceiling**

Cap the maximum number of ECS instances to prevent unbounded scaling from
overwhelming the database, even with small pools:

```yaml
metadata:
  annotations:
    run.googleapis.com/maxInstances: "100"
```

This creates a hard upper bound:
- **Guaranteed maximum: 500 connections** (100 instances × 5 per pool)
- Size the Cloud SQL instance accordingly

**Layer 3: Cloud SQL Managed Connection Pooling (production upgrade)**

For production workloads where the real HSS replaces the mock and the ECS is the
only DB client, Cloud SQL Enterprise Plus edition offers Managed Connection
Pooling — a built-in PgBouncer-compatible pooler on port 6432 that multiplexes
thousands of application connections onto a small number of database connections:

```
100+ Cloud Run ECS instances (hundreds of app connections)
    |
    v
Cloud SQL Managed Connection Pooling (port 6432, transaction mode)
    |
    v
PostgreSQL backend (~50 actual database connections)
```

In transaction mode, a database connection is only held for the duration of a
transaction, then returned to the pool. Since our queries are short (entitlement
lookups, token inserts), each transaction holds a connection for milliseconds.

To enable, the ECS connects to port 6432 instead of 5432 — a `DATABASE_URL`
config change, no code changes:

```env
# Without managed pooling (direct)
DATABASE_URL=postgresql://ecs:pass@PRIVATE_IP:5432/entitlements

# With managed pooling (multiplexed)
DATABASE_URL=postgresql://ecs:pass@PRIVATE_IP:6432/entitlements
```

**Note:** Managed Connection Pooling requires Cloud SQL Enterprise Plus edition.
For the POC on the standard tier, Layers 1 + 2 (small pools + maxInstances)
are sufficient. Layer 3 is the production upgrade path.

**Connection budget:**

| Setting | Value |
|---------|-------|
| `DB_POOL_SIZE` (default) | 5 |
| Max ECS instances | 100 |
| **Theoretical max ECS connections** | **500** |
| Headroom (migrations, admin, monitoring) | ~50 |
| **Cloud SQL instance requirement** | **≥550 max_connections** |

A `db-custom-2-7680` instance supports 500 connections by default. To
accommodate the full theoretical max + headroom, either:
- Increase `max_connections` on Cloud SQL to 600 (via database flags), or
- Lower `DB_POOL_SIZE` to 4 at deployment time (100 × 4 = 400 + 50 = 450 < 500)

The right values depend on the Cloud SQL instance tier — this is why both
`DB_POOL_SIZE` and `maxInstances` are configurable, not hardcoded.

### 15.5 Networking

```
Internet (untrusted)
    |
    v
Cloud Armor (DDoS protection + WAF rules)
    |
    v
Global External Application Load Balancer (HTTPS, managed TLS cert)
    |
    v
Cloud Run: entitlement-server (HTTP :8080, public via LB)
    |  (Direct VPC Egress — instances sit on VPC subnet, no proxy)
    |
    ├──> Cloud SQL (PostgreSQL, private IP)
    ├──> Memorystore (Redis, private IP)
    └──> Cloud Run: mock-hss (HTTP :3001, internal-only ingress)
              |
              ├──> Cloud SQL (same instance, reads ki_encrypted)
              └──> Cloud KMS (unwrap DEKs — IAM scoped to mock-hss only)
```

- **Cloud Armor** sits in front of the load balancer, inspecting all traffic before it reaches the ECS
- ECS connects to Cloud SQL, Memorystore, and mock HSS over **private IP** via Direct VPC Egress
- Mock HSS ingress is set to **internal-only** — unreachable from the internet
- KMS IAM role (`roles/cloudkms.cryptoKeyDecrypter`) is granted only to the mock HSS service account, not the ECS
- No database, Redis, or mock HSS port is exposed to the internet
- Cloud Run's built-in ingress controls restrict ECS traffic to the load balancer

### 15.6 DDoS Protection & WAF (Cloud Armor)

The ECS is internet-facing and will receive traffic from millions of devices. Cloud
Armor is attached to the Global External Application Load Balancer as a **security
policy** — all traffic passes through it before reaching Cloud Run.

**DDoS Protection:**

- **Cloud Armor Standard** (included with LB) provides always-on volumetric DDoS
  mitigation at L3/L4 — absorbs SYN floods, UDP reflection, and amplification attacks
  at Google's edge network before traffic reaches the LB
- **Cloud Armor Managed Protection Plus** (optional upgrade) adds adaptive protection
  that uses ML to detect and mitigate L7 application-layer DDoS (e.g., HTTP floods
  targeting `/entitlement`) with automatic rule tuning

**WAF Rules (Cloud Armor security policy):**

| Rule | Priority | Description |
|------|----------|-------------|
| Rate limiting | 1000 | Throttle per-IP request rate (e.g., 100 req/min per IP). Devices authenticate infrequently — sustained high rates from a single IP indicate abuse. |
| Geo-restriction | 2000 | Allow only countries where the operator has subscribers. Block traffic from regions with no legitimate device base. |
| OWASP ModSecurity CRS | 3000 | Enable Cloud Armor's **preconfigured WAF rules** based on the OWASP ModSecurity Core Rule Set — covers SQL injection, XSS, RCE, protocol attacks. These complement TypeBox schema validation as defense-in-depth. |
| Bot management | 4000 | Challenge or block requests with no valid `User-Agent` header or with known bot signatures. TS.43 devices send a specific `PRD-TS43/...` User-Agent — requests without it are suspicious. |
| Request size limit | 5000 | Reject requests with body > 8KB. Entitlement requests are small JSON payloads — oversized bodies indicate payload-based attacks. |
| Default allow | 65534 | Allow all remaining traffic that passes the above rules. |

**Rate limiting detail:**

Cloud Armor rate limiting is applied per security policy rule, using `rate_limit_options`:

```yaml
# Example: throttle per client IP
rule:
  action: throttle
  rate_limit_options:
    rate_limit_threshold:
      count: 100
      interval_sec: 60
    conform_action: allow
    exceed_action: deny(429)
    enforce_on_key: IP
```

This is especially important for the EAP-AKA path — each authentication attempt
triggers a KMS call and MILENAGE computation in the mock HSS. Without rate limiting,
an attacker could drive up KMS costs and exhaust mock HSS resources.

**Why Cloud Armor instead of application-level rate limiting:**

- Attacks are absorbed at Google's edge network, not at the Cloud Run instance
- Volumetric DDoS never reaches the application — Cloud Run doesn't scale up
  (and bill you) to absorb junk traffic
- WAF rules run before TLS termination overhead, before request parsing, before
  any application code
- Centralized policy management — one security policy for all rules, auditable
  via Cloud Logging

### 15.7 Code Changes for GCP

These are minimal and handled through the config layer:

**1. Config loader with Secret Manager fallback:**

```typescript
// src/config/index.ts
// In production: fetch from Secret Manager
// In development: read from .env / environment variables
async function loadConfig(): Promise<Config> {
  if (process.env.NODE_ENV === 'production') {
    const secrets = await loadFromSecretManager([
      'DATABASE_URL', 'REDIS_URL'
    ]);
    // Note: Ki encryption keys are managed by Cloud KMS, not Secret Manager.
    // KMS key references (ring, name, location) are non-secret config.
    return { ...defaults, ...secrets };
  }
  return { ...defaults, ...process.env };
}
```

**2. TLS conditional:**

```typescript
// src/index.ts
const app = buildApp(); // returns Fastify instance

if (config.TLS_ENABLED) {
  // Local dev: HTTPS with self-signed certs
  await app.listen({ port: config.PORT, host: '0.0.0.0', https: { cert, key } });
} else {
  // Cloud Run: plain HTTP (LB handles TLS)
  await app.listen({ port: config.PORT, host: '0.0.0.0' });
}
```

**3. Cloud Logging severity mapping:**

Pino's JSON output is already compatible with Cloud Logging. We add a `severity` field mapper:

```typescript
const pinoCloudLogging = {
  formatters: {
    level(label: string) {
      // Cloud Logging expects "severity" not "level"
      const severityMap: Record<string, string> = {
        trace: 'DEBUG', debug: 'DEBUG', info: 'INFO',
        warn: 'WARNING', error: 'ERROR', fatal: 'CRITICAL'
      };
      return { severity: severityMap[label] || 'DEFAULT' };
    }
  }
};
```

### 15.8 Deployment Pipeline

```
git push
    |
    v
Cloud Build trigger
    |
    ├── docker build → push to Artifact Registry
    └── gcloud run deploy --image ...
         |
         ├── Run DB migrations (Cloud Build step, via Cloud SQL proxy)
         └── Deploy new revision (zero-downtime, traffic splitting)
```

### 15.9 What Stays the Same

These require **zero changes** between local and GCP:
- Drizzle ORM queries (same PostgreSQL wire protocol)
- ioredis client (same Redis protocol, different host)
- All EAP-AKA logic, token management, entitlement handlers
- Fastify hooks and plugin pipeline
- XML/JSON response builders
- Docker as the packaging format

---

## 17. Key Technical Decisions & Trade-offs

| Decision | Rationale |
|----------|-----------|
| TypeScript over Go/Rust | Faster development for POC; team familiarity; rich npm ecosystem for crypto |
| Fastify over Express | **Schema-based validation**: Fastify validates request bodies against JSON Schema natively — TS.43 has well-defined parameter sets per AppID, so this replaces hand-written validation middleware. **Compiled serialization**: Fastify compiles JSON schemas into fast serializers at startup; every response is a structured entitlement config, so this matters. **Plugin encapsulation**: Each AppID handler (ap2003–ap2016) maps cleanly to a Fastify plugin with its own routes, hooks, and decorators — cleaner than Express router nesting. **TypeScript-first**: Fastify's type inference from schemas is stronger than Express's Request/Response generics. **Performance**: ~2-3x higher throughput than Express in benchmarks; during mass re-authentication events (network failover), lower per-request overhead is a real advantage. Express's ecosystem edge is irrelevant here — we don't use passport, session middleware, or any Express-specific packages. |
| TypeBox over Zod | Zod is a runtime interpreter — it re-evaluates schemas on every request. Using Zod with Fastify either bypasses Fastify's compiled Ajv pipeline (losing the core performance advantage) or requires an adapter that converts Zod→JSON Schema under the hood (adding a translation layer for no benefit). TypeBox outputs native JSON Schema directly, so Fastify compiles it into Ajv validators at startup with zero translation overhead. TypeBox also powers Fastify's response serialization via `fast-json-stringify`, which Zod cannot do. Single source of truth: `Static<typeof schema>` infers TypeScript types from the same schema object — no duplicate interface definitions to maintain. |
| PostgreSQL for relational data | Multi-table relational data with concurrent access; JSONB for flexible config |
| Redis for sessions | EAP-AKA sessions are short-lived and need fast lookup; natural fit & team familiarity |
| Dual XML+JSON responses | Spec requires both; abstract behind a common response builder |
| Mock HSS as separate service | Ki must never live in the internet-facing ECS. Separate internal-only service mirrors production topology (ECS calls HSS for vectors); KMS IAM scoped to mock HSS only; ECS is HSS-agnostic — it calls `HSS_URL` and gets vectors back. Migrating to a real HSS is a config change (`HSS_URL`), not a code change. |
| Own MILENAGE over npm package | The existing `milenage` npm package has 7 stars, no TypeScript types, no security audit, and is unmaintained. MILENAGE is well-specified (3GPP TS 35.206) and the only crypto primitive is AES-128, which we delegate to Node.js `crypto`/OpenSSL. Correctness is provable via official 3GPP TS 35.207/35.208 test vectors. No third-party crypto dependency in the critical path. |
| Monolith over microservices | POC scope; all 13 services in one process; easy to split later if needed |
| Docker Compose for local | Zero-install local dev with Postgres + Redis; CI-friendly |
| Cloud Run over GKE | No cluster management overhead; stateless HTTP server is a perfect fit |
| Direct VPC Egress over VPC Connector | VPC Connectors are legacy — they provision intermediate `e2-micro` proxy VMs that add latency, have throughput limits, and cost extra. Direct VPC Egress places Cloud Run instances directly on the VPC subnet with no proxy in the path. Lower latency to Memorystore/Cloud SQL, no connector to provision or scale, no extra cost. |
| Small pool + maxInstances + managed pooling | Three-layer defense against Cloud SQL connection exhaustion on the ECS. Layer 1: `DB_POOL_SIZE=5` per instance (configurable at deployment, not the library default of 10). Layer 2: `maxInstances: 100` cap on Cloud Run (100 × 5 = 500 max connections). Both are exposed as deployment-time parameters so operators can tune the product `maxInstances × DB_POOL_SIZE ≤ max_connections` for their Cloud SQL tier. Layer 3: Cloud SQL Managed Connection Pooling on port 6432 for production scale (Enterprise Plus, multiplexes hundreds of app connections to ~50 DB connections). POC uses Layers 1+2; Layer 3 is a config-only upgrade. |
| minInstances: 1 over scale-to-zero | EAP-AKA is a multi-round-trip protocol with a chained dependency (ECS → mock HSS). A fully cold start stacks up to 3-6s before the first response. `minInstances: 1` on both services ensures at least one warm container is always ready. Cost is minimal (idle CPU billing) vs. the risk of first-request timeouts. |
| OpenTelemetry over proprietary agents | Vendor-neutral instrumentation (OTLP) with GCP-native exporters. Auto-instrumentation patches `http`, `pg`, and `ioredis` transparently — no application code changes. If we ever move off GCP, swap the exporter, keep the instrumentation. Traces, metrics, and logs are correlated via trace ID for single-pane debugging. |
| Cloud Armor over app-level rate limiting | DDoS and WAF must be handled at the network edge, not in application code. Cloud Armor absorbs volumetric attacks at Google's edge before traffic reaches Cloud Run — preventing both service disruption and cost spikes from auto-scaling to absorb junk traffic. Preconfigured OWASP CRS rules add defense-in-depth alongside TypeBox schema validation. |
| Cloud SQL over self-managed | Managed backups, HA, patching; same PostgreSQL driver |
| Memorystore over self-managed | VPC-secured Redis with zero ops; same ioredis client |
| Secret Manager over env vars | Secrets never touch disk or container image; audit trail on access |
| Cloud Build over GitHub Actions | Native GCP integration; direct push to Artifact Registry and Cloud Run |
