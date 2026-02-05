# FOR-Hossein: Entitlements-as-a-Service, Explained

## What Is This Project?

Imagine you just bought a new phone and popped in your SIM card. You open the dialer and try to make a Wi-Fi call. Before the call connects, your phone silently asks your carrier: *"Is this person allowed to use Wi-Fi Calling?"* The carrier's server checks your subscription, checks your device, and sends back either a green light with configuration details, or a polite refusal.

That silent conversation is what this project implements. It's called an **Entitlement Configuration Server (ECS)** — the carrier-side system that answers the question *"What is this device allowed to do?"* for 12 different telecom services: Wi-Fi Calling, VoLTE, SMS over IP, eSIM activation, data plans, carrier billing, satellite connectivity, and more.

The protocol that governs this conversation is **GSMA TS.43** — a 200+ page telecom standard. Our job is to build a server that speaks this protocol fluently.

---

## The Big Picture: How Everything Connects

Think of the system as a fortress with a gatehouse and a vault.

**The Gatehouse** is the Entitlement Configuration Server (ECS) — the internet-facing service that talks to millions of phones. It handles HTTP requests, runs the authentication protocol, and serves entitlement configurations. It's the part of the system that attackers can see and probe.

**The Vault** is the Mock HSS — a separate, internal-only service that holds the crown jewels (subscriber secret keys called "Ki"). It's locked away behind the VPC with no internet access. The gatehouse can ask it questions ("Give me authentication vectors for this subscriber") but can never open the vault itself.

Here's how a request flows through the system:

```
                          THE INTERNET
                               |
                               v
                    +----- Cloud Armor -----+
                    |  DDoS protection      |
                    |  WAF rules            |
                    |  Rate limiting         |
                    +-----------+-----------+
                                |
                                v
                    +--- Load Balancer ----+
                    |  HTTPS termination   |
                    |  Managed TLS certs   |
                    +----------+-----------+
                               |
         +=====================|======================+
         |        VPC (Private Network)               |
         |                     |                      |
         |                     v                      |
         |     +---------- ECS -----------+           |
         |     |  Fastify HTTP server     |           |
         |     |  EAP-AKA state machine   |           |
         |     |  Token management        |           |
         |     |  12 service handlers     |           |
         |     |  TypeBox validation      |           |
         |     +--+--------+--------+----+           |
         |        |        |        |                 |
         |        v        v        v                 |
         |     Redis    Postgres  Mock HSS            |
         |   (sessions,  (tokens,  (internal only)    |
         |    tokens)   entitle-    |                  |
         |              ments)      v                  |
         |                       Postgres  Cloud KMS   |
         |                      (encrypted  (KEK for   |
         |                        Ki/OP)    unwrapping) |
         |                                             |
         +=============================================+
```

---

## What Phase 1 Built (The Foundation)

Phase 1 is the skeleton — the foundation that everything else builds on. Here's what exists right now:

### The Codebase Structure

```
src/
├── index.ts                 # Entry point: loads tracing FIRST, then starts server
├── config/
│   ├── index.ts             # Typed config from env vars (single source of truth)
│   ├── constants.ts         # Protocol constants, AppIDs, EAP-AKA attributes
│   ├── logger.ts            # Pino logger with Cloud Logging severity mapping
│   ├── tracing.ts           # OpenTelemetry distributed tracing (auto-instruments pg, redis, http)
│   └── metrics.ts           # OpenTelemetry metrics (meter provider for custom metrics)
├── db/
│   ├── schema.ts            # Drizzle ORM: 5 tables (subscribers, devices, entitlements, tokens, audit_log)
│   ├── index.ts             # PostgreSQL pool + Drizzle instance
│   └── redis.ts             # ioredis client with lazy connect + error handling
├── protocol/
│   ├── appIds.ts            # 12 service types (VoWiFi, VoLTE, eSIM, etc.)
│   ├── statusCodes.ts       # Entitlement/service/provisioning/TC status enums
│   ├── requestSchemas.ts    # TypeBox schemas for POST body + GET query validation
│   └── requestTypes.ts      # TypeScript types derived from schemas (never drift)
└── server/
    ├── app.ts               # Fastify builder: hooks → error handler → routes → warmup
    ├── routes/
    │   ├── health.ts        # GET /health → { status: "ok" }
    │   └── entitlement.ts   # GET & POST /entitlement → 501 (placeholder)
    └── middleware/
        ├── requestParser.ts # Normalizes GET query / POST body into uniform shape
        ├── userAgent.ts     # Parses "PRD-TS43/2 (Apple; iPhone15Pro; Smartphone; iOS18)"
        ├── versionCheck.ts  # Rejects unsupported entitlement_version → 406
        └── errorHandler.ts  # Centralized error responses with structured JSON
```

Plus: `Dockerfile` (multi-stage build), `docker-compose.yml` (Postgres + Redis + ECS), `drizzle.config.ts`, `package.json`, `tsconfig.json`.

---

## The Authentication Dance: EAP-AKA

The most technically interesting part of the system is how we prove a device is who it claims to be, without the device ever sending its secret key over the network. This is the **EAP-AKA protocol** (Extensible Authentication Protocol — Authentication and Key Agreement), and it's beautifully elegant.

### The Analogy: The Challenge Coin

Imagine you and a friend both have identical rare coins. To prove your friend is real (not an impostor), you describe a specific scratch pattern on your coin and ask: "What's on the other side of that scratch?" Only someone holding the real coin could answer correctly. Neither of you ever had to show the actual coin.

That's EAP-AKA. The SIM card in your phone and the carrier's HSS (Home Subscriber Server) both know a 128-bit secret key called **Ki**. The server sends a random challenge; the SIM card performs cryptographic math with Ki to produce a response. The server performs the same math and checks if the answers match. Ki never travels across the network.

### The HTTP Dance (2 Round-Trips)

```
Phone                              ECS                        Mock HSS
  |                                 |                            |
  |  "I want VoWiFi config"        |                            |
  |-------- POST /entitlement ----->|                            |
  |                                 |--- "Vectors for IMSI X" -->|
  |                                 |<-- RAND, AUTN, XRES, CK --|
  |                                 |                            |
  |<------- 401 + Challenge --------|                            |
  |   "Prove you have the SIM"     |                            |
  |                                 |                            |
  |  SIM card computes response     |                            |
  |-------- POST /entitlement ----->|                            |
  |   "Here's my proof (RES)"      |                            |
  |                                 |--- Checks: RES == XRES?   |
  |                                 |--- YES! Authenticated.     |
  |                                 |                            |
  |<------- 200 OK + Config --------|                            |
  |   Token + VoWiFi addresses      |                            |
```

After this dance, the device gets a **token** that it presents on future requests — skipping the whole authentication ceremony. We call this "fast auth," and it's why your phone doesn't have to re-authenticate every time it checks entitlements.

---

## The Technology Stack (And Why Each Choice Was Made)

### Fastify Over Express

Express is the default choice for Node.js HTTP servers, but we picked **Fastify** for specific reasons that matter for this project:

1. **Schema-based validation**: TS.43 defines precise parameter sets for every request. Fastify validates incoming JSON against compiled schemas *before* our code runs. A malformed IMEI or invalid AppID never reaches the handler.

2. **Compiled serialization**: Fastify compiles JSON schemas into fast serializers at startup. Since every response is a structured entitlement config, this matters.

3. **Plugin encapsulation**: Each of the 12 service handlers (VoWiFi, VoLTE, ODSA, etc.) maps naturally to a Fastify plugin with its own routes and hooks.

### TypeBox Over Zod

Both define validation schemas in TypeScript. We picked **TypeBox** because it outputs native JSON Schema, which Fastify compiles into Ajv validators at startup. Zod is a runtime interpreter — it re-evaluates schemas on every single request. TypeBox also gives us a single source of truth: `Static<typeof schema>` infers TypeScript types from the validation schema, so they can never drift apart.

### PostgreSQL + Redis (Not One or the Other)

Different data has different lifetimes and access patterns:

- **Subscriber records, entitlements, and tokens** live in **PostgreSQL**. They need durability, relational integrity, and auditability.
- **EAP-AKA sessions** live in **Redis only**. They last ~90 seconds, need sub-millisecond lookup, and losing one just means the device re-authenticates.
- **Auth tokens** use **both**: PostgreSQL is the durable source of truth; Redis is a fast-path cache (write-through pattern).

### Drizzle ORM

Lightweight, type-safe, and stays close to SQL. The schema in `src/db/schema.ts` defines 5 tables that map directly to the SQL in the spec. You can always drop down to raw SQL when needed — Drizzle doesn't try to hide the database from you.

---

## Security Architecture: Thinking Like a Paranoid Engineer

### Why the Mock HSS Is a Separate Service

The most impactful architectural decision was making the Mock HSS a separate service. The ECS is internet-facing. If it's compromised and it holds Ki, the attacker gets everything — every subscriber's identity.

So we made a rule: **the ECS never touches Ki.** The ECS calls the Mock HSS over the internal VPC and gets back derived vectors (RAND, AUTN, XRES, CK, IK). It has no way to reconstruct Ki from these — that's a mathematical property of the MILENAGE algorithm.

### Envelope Encryption

Ki is encrypted at rest using **envelope encryption**: each subscriber has their own Data Encryption Key (DEK), and all DEKs are wrapped by a Key Encryption Key (KEK) in Google Cloud KMS hardware. The KEK never leaves KMS. You need both the wrapped DEKs *and* KMS access to decrypt anything.

---

## Lessons From Phase 1

### 1. Import Order Matters for OpenTelemetry

The very first line of `src/index.ts` is `import './config/tracing.js'`. This isn't decorative — OpenTelemetry works by monkey-patching modules (`http`, `pg`, `ioredis`) *before* they're imported. If you import `pg` before initializing OTel, the pg instrumentation silently does nothing. This is a common gotcha that causes engineers to think "tracing is broken" when really it's just loaded too late.

**Takeaway:** When a library works by patching other modules, initialization order is part of your correctness contract. Document it. Enforce it.

### 2. The `Redis.default` ESM Constructor Trap

When using ioredis with ESM (`"type": "module"` in package.json), `new Redis(...)` fails with "not constructable." The fix is `new Redis.default(...)`. This is a classic ESM/CJS interop issue — the module's default export gets wrapped in a namespace object. This bug produces a confusing error message that gives no hint about the actual cause.

**Takeaway:** ESM/CJS interop in Node.js is still rough. When a constructor "isn't constructable," check if you need `.default`. This is especially common with `ioredis`, `pg`, and other packages that haven't fully migrated to ESM.

### 3. Schema-First Design Eliminates Type Drift

The TypeBox schemas in `requestSchemas.ts` are the single source of truth. The TypeScript types in `requestTypes.ts` are *derived* from those schemas via `Static<typeof ...>`. If the schema says `terminal_id` is 14-16 characters, the TypeScript type inherits that constraint. One definition, two uses, zero drift.

**Takeaway:** "Define once, derive everywhere" is a powerful pattern. Whenever you find yourself maintaining two parallel definitions (validation rules + TypeScript types, database schema + API types), look for a way to derive one from the other.

### 4. Fastify Hooks vs Express Middleware

Fastify doesn't use `app.use()`. Instead, it has lifecycle hooks (`onRequest`, `preValidation`, `preHandler`, etc.) that fire at specific points. Our hooks run in order: parse the request → parse the User-Agent → check the version. If the version check fails (406), it fires *before* Fastify's schema validation, saving CPU on obviously invalid requests.

**Takeaway:** Understanding your framework's request lifecycle — not just "middleware runs in order" but *exactly when* each hook fires relative to validation — lets you fail fast and cheaply.

### 5. Lazy Connections Prevent Startup Crashes

Redis uses `lazyConnect: true`, meaning it doesn't try to connect when instantiated. Instead, it connects during Fastify's `onReady` hook. This prevents a race condition where Redis connection errors crash the process before error handlers are registered.

**Takeaway:** Any connection that can fail should be deferred to a lifecycle hook where you can handle the failure gracefully. Don't let a constructor crash your process.

### 6. Multi-Stage Docker Builds: Smaller + Safer

The Dockerfile has two stages: `builder` (all deps, compiles TS) and `runner` (production deps only, compiled JS). The final image doesn't contain TypeScript source, dev dependencies, or build tools. This shrinks the image and reduces the attack surface.

**Takeaway:** Always use multi-stage builds for compiled languages. The build environment should never ship to production.

### 7. Why 501, Not 404

The entitlement routes return `501 Not Implemented`, not `404 Not Found`. 404 means "this route doesn't exist." 501 means "I understand your request, but I haven't built the logic yet." This distinction matters when incrementally building a system — you can tell the difference between "wrong URL" and "not yet coded."

**Takeaway:** HTTP status codes have precise meanings. Using the right one makes debugging easier and makes your API self-documenting.

### 8. Config as a Typed Object, Not Scattered `process.env`

`src/config/index.ts` reads all environment variables once, applies defaults, and exports a typed `Config` object. No other file reads `process.env` directly. Benefits: tests can override config, the app starts without a `.env` file, and every config value has exactly one place where it's defined.

**Takeaway:** Scattering `process.env.THING` throughout a codebase leads to typos, missing defaults, and untestable code. Centralize config into a typed object at the boundary.

---

## Phase 3: EAP-AKA Authentication — The Core Protocol

Phase 3 is where the project comes alive. The 501 stubs are gone, replaced by a working two-round-trip EAP-AKA challenge-response flow. Here's what was built:

### New Files in `src/auth/`

```
src/auth/
  eapCodec.ts          — EAP-AKA packet binary encoder/decoder (RFC 4187)
  keyDerivation.ts     — SHA-1 PRF + HMAC-MAC key derivation
  eapAkaVectors.ts     — HTTP client that calls the mock HSS
  eapSession.ts        — Redis session management (90s TTL)
  tokenService.ts      — Token generation + validation (Postgres + Redis)
  eapIdempotency.ts    — Response replay cache for retried requests
  eapAka.ts            — The orchestrator: ties all six modules together
```

Plus modifications to `src/server/routes/entitlement.ts` and 3 test files.

### How the Code Flows

When a phone hits `POST /entitlement`, the route handler does three-path routing:

1. **Token present?** → Validate via Redis/Postgres → return entitlements
2. **eap_relay present?** → Process EAP-AKA response → issue token → return entitlements
3. **Neither?** → Require IMSI → challenge the device → return 401

The orchestrator (`eapAka.ts`) coordinates the flow:

**Round Trip 1 (Challenge):**
```
fetchVectors(imsi)           → call mock HSS for RAND, AUTN, XRES, IK, CK
buildIdentity(imsi)          → "0" + imsi (permanent identity format)
deriveKeys(identity, ik, ck) → MK = SHA-1(id|IK|CK) → PRF → K_encr, K_aut, MSK, EMSK
encodeEapPacket(challenge)   → binary packet with AT_RAND, AT_AUTN, zeroed AT_MAC
computeMac(kAut, packet)     → HMAC-SHA-1 truncated to 16 bytes
patch MAC into packet bytes  → find AT_MAC offset, copy real MAC value in
createSession(redis)         → store XRES, K_aut, state=CHALLENGE_SENT, 90s TTL
return 401 + eap_relay + X-EAP-Session-Id header
```

**Round Trip 2 (Response):**
```
getSession(sessionId)         → retrieve from Redis (or fail if expired)
decodeEapPacket(eap_relay)    → parse the device's EAP-Response
check subtype                 → handle AUTH_REJECT, SYNC_FAILURE, wrong type
timingSafeEqual(AT_RES, XRES) → verify the SIM's answer (constant-time!)
verifyMac(kAut, packet, mac)  → verify the packet wasn't tampered with
generateToken(subscriberId)   → crypto.randomBytes(32) → Postgres + Redis
deleteSession(sessionId)      → clean up Redis
return 200 + token + EAP-Success
```

### Lessons From Phase 3

#### 1. Binary Protocol Encoding: Alignment Bites

The EAP-AKA packet format requires every attribute to be padded to **4-byte boundaries**, with the "length" field counting in 4-byte words. Our first AT_RES encoding had a subtle bug:

```typescript
// BUG: padded the value payload independently of the 2-byte header
const paddedLen = Math.ceil((2 + attr.value.length) / 4) * 4;
// Gave 12 bytes of payload → total = 2 (header) + 12 = 14 bytes. Not aligned!

// FIX: pad the ENTIRE attribute (including header) to multiple of 4
const totalAttrLen = Math.ceil((2 + 2 + attr.value.length) / 4) * 4;
valuePayload = Buffer.alloc(totalAttrLen - 2);
```

**Takeaway**: When working with binary protocols, think about alignment from the total structure's perspective, not individual fields. Draw the byte layout on paper first.

#### 2. Timing-Safe Comparisons Are Non-Negotiable

When comparing AT_RES to XRES, we use `crypto.timingSafeEqual()` instead of `===` or `Buffer.equals()`. Regular comparison stops at the first mismatched byte — an attacker could measure response times and gradually guess the correct value.

```typescript
// WRONG — leaks timing information
if (atRes.value.equals(expectedXres)) { ... }

// RIGHT — constant-time comparison
if (crypto.timingSafeEqual(atRes.value, expectedXres)) { ... }
```

**Takeaway**: Any comparison of secret values (tokens, MACs, passwords, crypto outputs) must be timing-safe. This is a one-line fix that prevents a real attack class.

#### 3. The FIPS 186-2 PRF: Grade-School Addition at 160 Bits

The key derivation PRF treats a 20-byte buffer as a single 160-bit big-endian integer and does modular addition. We had to implement carry-propagating addition across 20 bytes:

```typescript
function add160(a: Buffer, b: Buffer): void {
  let carry = 0;
  for (let i = 19; i >= 0; i--) {  // right-to-left, just like adding by hand
    const sum = a[i]! + b[i]! + carry;
    a[i] = sum & 0xff;
    carry = sum >> 8;
  }
}
```

**Takeaway**: Crypto algorithms often treat byte arrays as big numbers. Understanding endianness (3GPP is always big-endian) is crucial for correctness.

#### 4. The MAC-Over-Zeroed-MAC Pattern

To compute AT_MAC, you include the MAC attribute in the packet (so the length is correct) but zero its value first. Then you patch the real MAC in afterward:

```typescript
// 1. Build packet with zeroed MAC field
const packet = encodeEapPacket({ ..., attributes: [..., { type: AT_MAC, value: Buffer.alloc(16) }] });
// 2. Compute MAC over the entire packet (with zeroed MAC)
const mac = computeMac(kAut, packet);
// 3. Patch real MAC into the packet bytes at the right offset
mac.copy(packet, macOffset + 4);
```

If you compute the MAC without the MAC attribute present, the packet length differs, and verification on the other side fails silently.

**Takeaway**: Read the RFC carefully. "The MAC is calculated over the EAP packet with the MAC field set to zero" means the field must *exist* but be zeroed — not absent.

#### 5. Idempotency Prevents Double-Token-Generation

Network requests get retried. Without idempotency, Round Trip 2 could try to consume a session that's already been deleted, causing a failure. Our solution: cache successful responses for 90 seconds, keyed by `SHA-256(sessionId + eapRelay)`.

**Takeaway**: Any state-mutating operation should be idempotent. Cache the result and replay it for duplicate requests.

#### 6. Error Cases Outnumber Happy Paths 10:1

The `handleEapResponse()` function handles 10 distinct error cases before reaching the success path. Each one: logs the issue, cleans up the session, returns EAP-Failure. This is normal — good engineers spend more time thinking about failure modes than success paths.

---

## Phase 6: ODSA — Teaching the Server to Hand Out eSIMs

### What Is ODSA?

Everything we've built so far — VoWiFi, VoLTE, SMSoIP — is essentially a lookup: "Is this user allowed to use this service? Here's the config." ODSA (On-Device Service Activation) is fundamentally different. It's a **stateful workflow**: a device checks if it's eligible for an eSIM, subscribes to a plan, and then downloads an eSIM profile. Think of VoWiFi as checking your library card, while ODSA is the whole process of applying for a card, choosing a membership tier, and getting it printed.

There are two flavors:
- **ap2006 (Companion Device)**: Your smartwatch wants its own phone number linked to your main phone's plan
- **ap2009 (Primary Device)**: Your new phone wants to download its own eSIM plan

### The Design Decision: ODSA Fits Inside the Existing Pipeline

The tempting approach was to build ODSA as a completely separate subsystem — new routes, new handlers, a whole parallel pipeline. Instead, we made a key architectural decision: **ODSA operations are just another dimension of the existing entitlement flow.**

The request schema already had an `operation` field. The entitlement table already had a `configData` JSONB column. The `ApplicationConfig` type already had an `extraParams` bag. ODSA plugs into all three:

1. The `operation` field acts as a **sub-router** within the service handler ("CheckEligibility", "ManageSubscription", "AcquirePlan", etc.)
2. The `configData` JSONB stores **workflow state** (`subscriptionState: 'eligible'`, `smdpAddress: 'smdp.operator.com'`)
3. `extraParams` carries **ODSA-specific response fields** (`SubscriptionResult`, `SMDP+Address`, `ProfileICCID`) through the JSON/XML builders unchanged

This means zero changes to the JSON builder, zero changes to the XML builder, zero changes to the response types. The new code just plugs in.

### The Mock SM-DP+: Pretending We Have an eSIM Platform

In the real world, when a subscriber downloads an eSIM profile, the entitlement server talks to an **SM-DP+** (Subscription Manager - Data Preparation) server. This is a complex system that manages profile packages, signs them cryptographically, and delivers them to devices via QR codes or activation codes.

We don't need any of that complexity for a POC. So we built `mockSmdp.ts` — a pure in-process module (not even a separate HTTP service) that returns three canned eSIM activation codes:

```
default  → "1$smdp.operator.com$POSTPAID-001"  (postpaid plan)
prepaid  → "1$smdp.operator.com$PREPAID-001"   (prepaid plan)
companion → "1$smdp.operator.com$COMPANION-001" (companion device)
```

The activation code format `1$address$matchingId` is the real TS.43 format. The `1` means "use SMDP+ protocol version 1." A real device would scan this code (or receive it over the air) and use it to contact the SM-DP+ and download its eSIM profile.

### SubscriptionResult: The State Machine Output

Every ODSA response includes a `SubscriptionResult` — a numeric code telling the device what to do next. These codes were originally defined as strings in our codebase, but TS.43 requires numeric values on the wire:

| Code | Name | Meaning |
|------|------|---------|
| 1 | CONTINUE_TO_WS | "Go to this web URL to complete the process" |
| 2 | DOWNLOAD_PROFILE | "Here's your eSIM activation code, download it" |
| 3 | DONE | "Nothing more to do" |
| 4 | DELAYED_DOWNLOAD | "Check back later" |
| 6 | DELETE_PROFILE_IN_USE | "Delete your current profile first" |
| 7 | REQUIRES_USER_INPUT | "We need more info from you" |

Notice there's no 5. The TS.43 spec skipped it. Don't ask why. Telecom standards are like that.

### The AcquireTemporaryToken Problem: When a Side Effect Crosses Layers

Most ODSA operations are pure: take config data in, produce a response config out. But `AcquireTemporaryToken` is different — it has a **side effect**: it needs to generate a new temporary token and inject it into the response.

The service handler (in `odsaCompanion.ts` / `odsaPrimary.ts`) is a pure function that transforms config data into an `ApplicationConfig`. It doesn't have access to the database, Redis, or the request context. Token generation requires all three.

The solution: **the service handler returns `DONE`, and the route layer handles the side effect.** After building the response, the route handler checks if the operation was `AcquireTemporaryToken`. If so, it generates the token and injects `TemporaryToken` and `TemporaryTokenValidity` into the response's app block.

This is a classic example of the "pure core, imperative shell" pattern. The service handlers stay testable without mocking databases. The route handler handles the messy real-world stuff.

### The File Map

```
src/services/
  mockSmdp.ts          — 3 canned eSIM profiles (not a network service, just a lookup table)
  odsaCommon.ts        — Shared types (OdsaConfigData, OdsaContext) + buildOdsaBaseConfig helper
  odsaCompanion.ts     — ap2006 handler: 6 operations (CheckEligibility, ManageSubscription, etc.)
  odsaPrimary.ts       — ap2009 handler: same 6 + AcquirePlan

src/protocol/
  statusCodes.ts       — SubscriptionResult changed from strings to TS.43 numeric codes
  requestSchemas.ts    — Added AcquirePlan to the OdsaOperationSchema union
  responseBuilder.ts   — Added ap2006/ap2009 routing + optional OdsaContext parameter

src/auth/
  tokenService.ts      — Added generateTemporaryToken() for ODSA flows

src/server/routes/
  entitlement.ts       — Passes OdsaContext to response builder + AcquireTemporaryToken side effect

src/db/
  seed-entitlements.ts — 4 new ODSA seed records (Alice & Bob × companion & primary)
```

### Lessons From Phase 6

#### 1. Backward Compatibility Through Optional Parameters

The `buildEntitlementResponse()` function gained an `odsaContext` parameter, but it's optional. The three existing service handlers (VoWiFi, VoLTE, SMSoIP) don't know or care about it. None of their call sites changed. The new `buildAppConfig()` switch cases pass `odsaContext` to the ODSA handlers; the old ones ignore it.

**Takeaway:** When extending a pipeline, make new parameters optional with sensible defaults. Existing callers should work without changes.

#### 2. The extraParams Bag Is a Powerful Extension Point

We defined `extraParams: Record<string, string>` back in Phase 5 for VoLTE's `VoLTE_Entitled` and `VoNR_Entitled`. Now ODSA uses the same mechanism for `SubscriptionResult`, `SMDP+Address`, `SMDP+ActivationCode`, `ProfileICCID`, `ServiceFlow_URL`, `PlanId`, `PlanName`, `TemporaryToken`, and `TemporaryTokenValidity`. The JSON and XML builders iterate over `extraParams` without knowing what the keys mean.

**Takeaway:** A generic key-value bag, combined with typed handler functions that populate it, is more maintainable than adding typed fields for every possible response parameter. The bag is "untyped at the wire level, typed at the construction level."

#### 3. Sub-Routing via Operation Field

Instead of creating new HTTP endpoints (`/entitlement/companion/check-eligibility`), we used the existing `operation` field in the request body as a sub-router. This matches how TS.43 works: the device sends a single POST with different `operation` values.

**Takeaway:** When a protocol defines its own routing mechanism, use it. Don't fight the spec by mapping protocol operations to HTTP paths. Your API surface should mirror the protocol surface.

#### 4. Pure Handlers + Imperative Side Effects

The companion and primary handlers are pure functions: `(status, provStatus, tcStatus, configData, odsaContext) → ApplicationConfig`. They're trivially testable — no mocking needed. The 17 unit tests run in 9ms total. The side effect (token generation) lives in the route handler where it has access to the request context and database.

**Takeaway:** Keep business logic pure. Push I/O to the edges. Your unit tests will thank you.

## Phase 7: Extended Services — From 5 Services to 12

### What Changed

After Phase 6, the server handled 5 services: VoWiFi (ap2004), VoLTE (ap2003), SMSoIP (ap2005), ODSA Companion (ap2006), and ODSA Primary (ap2009). Phase 7 adds 7 more, bringing the total to 12 — covering nearly every service type in the TS.43 specification.

The new services fall into two categories: **operation-aware** (like ODSA, with sub-routing via the `operation` field) and **simple builders** (like VoWiFi, with just status-based logic).

### The New Services

| App ID | Service | Type | What It Does |
|--------|---------|------|--------------|
| ap2010 | Data Plan Info | Operation-aware | Check plan eligibility, acquire plans, get data usage details |
| ap2011 | Server-Initiated ODSA | Operation-aware | Enterprise-managed eSIM provisioning (same pattern as companion/primary, scoped to enterprise) |
| ap2012 | Direct Carrier Billing | Simple | Carrier billing entitlement — enabled/disabled with optional T&C acceptance flow |
| ap2013 | Private User Identity | Simple | Returns pseudonymized subscriber identity when enabled |
| ap2014 | Device/User Info | Operation-aware | Get phone number or subscriber details |
| ap2015 | App Authentication | Simple | Returns operator token endpoint URL for app-level auth |
| ap2016 | Satellite Mode | Simple | Returns PLMN allow/barred lists for satellite connectivity |

### Why This Phase Was Fast (And What That Tells You)

Phase 7 added 7 service handlers, seed data for 14 new entitlement records, 31 unit tests, and wired everything into the response router — and it all worked on the first try. No debugging. No surprises.

That's not because the code was trivial. It's because **Phases 1–6 built the right abstractions.** Every new service handler follows an identical pattern:

```typescript
export function buildXxxConfig(
  status: number,
  provStatus: number,
  tcStatus: number,
  configData?: unknown,
  odsaContext?: OdsaContext,  // only for operation-aware services
): ApplicationConfig {
  // Cast configData, route by operation (if applicable), populate extraParams
}
```

Adding a new service means:
1. Create a file with the builder function
2. Add one `case` statement in `responseBuilder.ts`
3. Add seed data
4. Write tests

No new interfaces, no new middleware, no database migrations, no new routes. The `extraParams` bag carries all service-specific response fields through the JSON/XML builders untouched. The TypeBox schema needed one new literal per app ID. That's it.

This is the payoff of the decisions made in earlier phases. The builder pattern, the `extraParams` bag, the operation-based sub-routing, the `configData` JSONB column — they were all designed to make this kind of extension trivial. **Good architecture isn't about the code you write; it's about the code you don't have to write later.**

### The Builder Pattern: Two Flavors

**Simple builders** (ap2012, ap2013, ap2015, ap2016) follow the VoWiFi pattern:

```typescript
// Direct Carrier Billing (ap2012) — decision matrix
if (status === EntitlementStatus.INCOMPATIBLE) {
  result.extraParams = { Message: 'Not available for this device.' };
} else if (status === DISABLED && tcStatus === REQUIRES_ACCEPTANCE && data.serviceFlowUrl) {
  result.serviceFlowUrl = data.serviceFlowUrl;  // redirect to T&C portal
}
```

Three possible states → three code paths → three tests. No operation routing needed.

**Operation-aware builders** (ap2010, ap2011, ap2014) follow the ODSA pattern:

```typescript
// Data Plan (ap2010) — operation sub-routing
switch (operation) {
  case 'CheckEligibility': return handleCheckEligibility(...);
  case 'AcquirePlan':      return handleAcquirePlan(...);
  case 'GetPlanDetails':   return handleGetPlanDetails(...);
  default:                 return basicStatus;
}
```

Each operation handler is a small pure function. The `buildOdsaBaseConfig()` helper (from `odsaCommon.ts`) stamps in the `SubscriptionResult` and any extra fields. Server-Initiated ODSA (`ap2011`) even reuses the mock SM-DP+ activation codes for profile downloads.

### The Seed Data Strategy

Every service gets seed data for both test subscribers:

- **Alice** gets "everything enabled with rich config" — full plan details, PLMN lists, pseudonyms, token endpoints. This exercises the happy path and verifies that all `extraParams` fields propagate correctly.
- **Bob** gets "a mix of enabled and disabled" — some services need T&C acceptance, some are completely off. This tests the decision branches and verifies that disabled services don't leak config data.

The pattern is intentional: one subscriber tests "everything works," the other tests "everything fails gracefully."

### The File Map

```
src/services/
  dataPlan.ts             — ap2010: CheckEligibility, AcquirePlan, GetPlanDetails
  serverOdsa.ts           — ap2011: CheckEligibility, ManageSubscription, ManageService
  directCarrierBilling.ts — ap2012: INCOMPATIBLE/DISABLED/ENABLED decision matrix
  privateUserIdentity.ts  — ap2013: pseudonym when enabled
  deviceUserInfo.ts       — ap2014: GetPhoneNumber, GetSubscriberInfo
  appAuthentication.ts    — ap2015: token endpoint when enabled
  satMode.ts              — ap2016: PLMN allow/barred lists when enabled

tests/unit/
  extendedServices.test.ts — 31 tests covering all 7 new builders
```

---

## The Integration Test Fix: A Lesson in Self-Sufficient Tests

### The Bug

After implementing Phase 7, we noticed 4 integration tests had been failing since Phase 6. They all returned `SubscriptionResult='3'` (DONE) instead of `'2'` (DOWNLOAD_PROFILE) or `'1'` (CONTINUE_TO_WS). The unit tests passed fine.

### The Investigation

The integration tests go through the full HTTP stack: `POST /entitlement` → route handler → database lookup → service handler → response. The unit tests call the service handlers directly with test data.

The root cause was embarrassingly simple: **the database was empty.** The ODSA seed data (ap2006, ap2009 entitlement records) was added to `seed-entitlements.ts` in Phase 6, but nobody ran the seed script against the database after that. Without entitlement records, `responseBuilder.ts` fell back to defaults: `configData=undefined`. And when the ODSA handlers got `undefined` config data, they cast it to `{}` — which has no `subscriptionState`, no `smdpAddress`, no `serviceFlowUrl` — so every operation fell through to the default `SubscriptionResult.DONE`.

The tricky part: the tests didn't crash. They got valid responses with the wrong data. Silent failures are the worst kind of failures.

### The Real Fix (Not Just Running the Seed)

Running `npx tsx src/db/seed-entitlements.ts` fixed the tests immediately. But that's not a fix — it's a band-aid. The next person to clone the repo, or the next time someone resets the database, the same tests would break again with no obvious explanation.

The real fix had three parts:

**1. Make the seed script importable.** Before, it was a standalone script that called `seed()` at the top level. We refactored it to export a `seedEntitlements()` function, with the top-level call gated behind a check for direct execution:

```typescript
export async function seedEntitlements(dbUrl?: string) { /* ... */ }

const isDirectRun = process.argv[1]?.includes('seed-entitlements');
if (isDirectRun) {
  seedEntitlements().catch(/* ... */);
}
```

**2. Create a vitest `globalSetup` hook.** The `tests/integration/setup.ts` file calls `seedEntitlements()` before any test file runs:

```typescript
import { seedEntitlements } from '../../src/db/seed-entitlements.js';

export async function setup() {
  await seedEntitlements(databaseUrl);
}
```

**3. Wire it up in `vitest.config.ts`:**

```typescript
export default defineConfig({
  test: {
    globalSetup: ['./tests/integration/setup.ts'],
  },
});
```

Now `npm test` seeds the database automatically. The seed uses `ON CONFLICT ... DO UPDATE`, so it's idempotent — safe to run every time. The integration tests are self-sufficient.

**4. Fix a hidden test landmine.** The "no entitlement for subscriber+app" test was using `ap2010` — which *did* have no entitlement for Alice before Phase 7, but now does. We changed it to use `ap2005` for Bob (who genuinely has no SMSoIP entitlement). And we couldn't use a made-up app ID like `ap2099` because the Fastify request schema validates `app` against a union of known app IDs, and would reject it with a 400 before it even reached the handler.

### Lessons From This Bug

#### 1. Integration Tests Must Own Their Data

Integration tests that depend on "someone ran a seed script at some point" are time bombs. Every test suite should be able to set up its own preconditions. Vitest's `globalSetup` is purpose-built for this — it runs once before all test files, not once per file.

**Takeaway:** If a test depends on database state, the test (or its setup) must create that state. Manual setup steps are a form of technical debt that compounds silently.

#### 2. Silent Failures Are Worse Than Crashes

The tests didn't crash — they got `SubscriptionResult='3'` instead of `'2'`. The assertions caught the difference, but without understanding the system, the error message "expected '2' to be '3'" gives no clue about the root cause. If the handlers had thrown an error on missing `configData` (e.g., `assert(data.subscriptionState, 'configData.subscriptionState required for ManageSubscription')`), the bug would have been obvious instantly.

**Takeaway:** When a function receives data from the database and silently falls through to a default, consider whether that default is actually correct for the context. Sometimes "fail loudly" is better than "degrade gracefully."

#### 3. Test Data Assumptions Rot Over Time

The test assumed Alice had no ap2010 entitlement. That was true when the test was written. Phase 7 added one. The test still passed (because the status value happened to match), but it was no longer testing what it claimed to test. Using a subscriber+app combo that's *structurally* guaranteed to have no record (Bob + ap2005, which is simply never seeded) is more robust than depending on "this particular app ID hasn't been added yet."

**Takeaway:** When writing a test for "missing data," choose test fixtures that are explicitly excluded from seed data — not ones that just happen to be absent today.

#### 4. Idempotent Seeds Are Infrastructure

The seed script's `ON CONFLICT ... DO UPDATE` clause is what makes this whole approach work. Without it, running the seed twice would crash with a unique constraint violation. With it, you can run it on every test invocation without worry. This pattern — "ensure state exists, creating or updating as needed" — is called **upsert**, and it's essential for any setup script that might run more than once.

**Takeaway:** Seed scripts should always be idempotent. Use `ON CONFLICT DO UPDATE` (Postgres), `INSERT ... ON DUPLICATE KEY UPDATE` (MySQL), or your ORM's equivalent.

---

## The Full Architecture (After Phase 7)

```
src/
├── index.ts                          # Entry point: OTel → dotenv → start server
├── config/
│   ├── index.ts                      # Typed config from env vars
│   ├── constants.ts                  # 12 AppIDs, HTTP codes, EAP constants, token types
│   ├── logger.ts                     # Pino logger with Cloud Logging severity
│   ├── tracing.ts                    # OpenTelemetry (HTTP, pg, ioredis instrumentation)
│   └── metrics.ts                    # OTel metrics provider
├── db/
│   ├── schema.ts                     # 5 tables: subscribers, devices, entitlements, tokens, audit_log
│   ├── index.ts                      # PostgreSQL pool + Drizzle ORM
│   ├── redis.ts                      # IORedis client (lazy connect)
│   └── seed-entitlements.ts          # Idempotent seed (23 records, exportable function)
├── auth/
│   ├── eapAka.ts                     # EAP-AKA state machine (2 round-trips)
│   ├── eapCodec.ts                   # Binary EAP packet encoder/decoder (RFC 4187)
│   ├── keyDerivation.ts              # SHA-1 PRF + HMAC-MAC (FIPS 186-2)
│   ├── eapAkaVectors.ts              # HTTP client → mock HSS
│   ├── eapSession.ts                 # Redis session store (90s TTL)
│   ├── eapIdempotency.ts             # Redis replay cache (90s TTL)
│   └── tokenService.ts              # Token CRUD + rotation + temporary tokens
├── protocol/
│   ├── requestSchemas.ts             # TypeBox: 12 app IDs, 7 ODSA operations
│   ├── requestTypes.ts               # Derived TypeScript types
│   ├── responseTypes.ts              # ApplicationConfig, ServiceEntitlementResponse
│   ├── statusCodes.ts                # Entitlement/Service/Prov/TC/SubscriptionResult codes
│   ├── appIds.ts                     # APP_ID constants
│   ├── responseBuilder.ts            # Router: appId → service handler → format
│   ├── jsonBuilder.ts                # TS.43 JSON formatter
│   └── xmlBuilder.ts                 # WAP-Provisioning XML formatter
├── services/                         # 12 service handlers
│   ├── vowifi.ts                     # ap2004 — Wi-Fi Calling
│   ├── volte.ts                      # ap2003 — VoLTE/VoNR
│   ├── smsoip.ts                     # ap2005 — SMS over IP
│   ├── odsaCommon.ts                 # Shared ODSA types + buildOdsaBaseConfig
│   ├── odsaCompanion.ts              # ap2006 — eSIM companion device
│   ├── odsaPrimary.ts                # ap2009 — eSIM primary device
│   ├── mockSmdp.ts                   # 3 canned eSIM activation codes
│   ├── dataPlan.ts                   # ap2010 — Data plan info
│   ├── serverOdsa.ts                 # ap2011 — Server-initiated ODSA
│   ├── directCarrierBilling.ts       # ap2012 — Carrier billing
│   ├── privateUserIdentity.ts        # ap2013 — Pseudonymized identity
│   ├── deviceUserInfo.ts             # ap2014 — Phone number + subscriber info
│   ├── appAuthentication.ts          # ap2015 — Operator token endpoint
│   └── satMode.ts                    # ap2016 — Satellite PLMN lists
└── server/
    ├── app.ts                        # Fastify setup + lifecycle hooks
    ├── routes/
    │   ├── entitlement.ts            # POST/GET /entitlement (3-path routing)
    │   └── health.ts                 # GET /health
    └── middleware/
        ├── requestParser.ts          # GET/POST normalization
        ├── userAgent.ts              # TS.43 User-Agent parsing
        ├── versionCheck.ts           # Version validation → 406
        └── errorHandler.ts           # Centralized error responses

tests/
├── unit/                             # 6 test files, ~120 tests, run in ~15ms
│   ├── eapCodec.test.ts
│   ├── keyDerivation.test.ts
│   ├── tokenService.test.ts
│   ├── responseBuilder.test.ts
│   ├── odsa.test.ts
│   └── extendedServices.test.ts
└── integration/                      # 2 test files, ~20 tests, require Docker
    ├── setup.ts                      # globalSetup: auto-seeds DB before tests
    ├── eapAka.integration.test.ts
    └── odsa.integration.test.ts

vitest.config.ts                      # globalSetup wiring
docker-compose.yml                    # Postgres + Redis + mock-hss + ecs
Dockerfile                            # Multi-stage (builder → runner)
```

**140 tests. 10 test files. All green.**

---

## Phase 8: GCP Infrastructure & CI/CD — Taking It to the Cloud

### The Transition

Up to Phase 7, the server ran in Docker Compose on a developer's laptop. Phase 8 answers the question every POC eventually faces: *"How do we deploy this for real?"*

We chose **Google Cloud Platform** with a specific opinionated stack:
- **Cloud Run** for containerized services (serverless, auto-scaling, pay-per-use)
- **Cloud SQL** for PostgreSQL (managed, private IP, automated backups)
- **Memorystore** for Redis (managed, VPC-connected)
- **Cloud KMS** for key management (hardware-backed, never exposes the KEK)
- **Cloud Build** for CI/CD (build, test, deploy on every push to main)
- **Cloud Armor** for WAF (rate limiting, SQL injection, XSS protection)
- **Terraform** for infrastructure-as-code (reproducible, reviewable, version-controlled)

### The Cloud KMS Upgrade: From XOR to Hardware Security

Remember the envelope encryption we built in Phase 2? The mock HSS stores subscriber Ki values encrypted with per-subscriber DEKs, and those DEKs are wrapped with a KEK. In development, the KEK is a hex string in an environment variable, and "wrapping" is just XOR.

In production, that's not acceptable. XOR is reversible — if an attacker gets the environment variable, they can unwrap every DEK. Cloud KMS solves this by storing the KEK in **tamper-proof hardware**. The key never leaves the HSM. The mock HSS sends a wrapped DEK to KMS and gets back the unwrapped DEK. Even if the VM is compromised, the attacker can't extract the KEK.

The clever part of our implementation: we made the `KeyManager` interface async-compatible:

```typescript
export interface KeyManager {
  unwrapDek(wrappedDek: Buffer): Buffer | Promise<Buffer>;
}
```

`LocalKeyManager.unwrapDek()` returns a synchronous `Buffer` (XOR is instant). `CloudKmsKeyManager.unwrapDek()` returns a `Promise<Buffer>` (network call to KMS). TypeScript's union type lets both satisfy the interface. The call site uses `await`, which works identically for both sync and async values.

The lazy import pattern is worth noting:

```typescript
async unwrapDek(wrappedDek: Buffer): Promise<Buffer> {
  if (!this.client) {
    const { KeyManagementServiceClient } = await import('@google-cloud/kms');
    this.client = new KeyManagementServiceClient();
  }
  // ...
}
```

By dynamically importing `@google-cloud/kms` only when first needed, we avoid requiring the GCP SDK in development environments where it's not installed.

### The Port 8080 Convention

A small but important change: the default port went from 8443 to 8080. Cloud Run expects containers to listen on the port specified by the `PORT` environment variable (which defaults to 8080). Using a non-standard port would require explicit configuration on every deployment and confuse anyone familiar with Cloud Run conventions.

### Connection Pool Tuning: Less Is More

The default `DB_POOL_SIZE` changed from 5 to 2. This seems counterintuitive — "wouldn't more connections be faster?" In a serverless environment, each Cloud Run instance is ephemeral. If you have 10 instances each holding 5 connections, that's 50 connections to Cloud SQL. Cloud SQL's `db-f1-micro` tier supports maybe 25 concurrent connections. You'd hit the limit and start getting connection refused errors.

With pool size 2, even at max scale (10 instances × 2 connections = 20), you stay well within limits. The lesson: **serverless connection pools should be small**. The platform scales horizontally; individual instances should be modest.

### Terraform: Infrastructure as Code

The `terraform/` directory contains 9 modules, each responsible for one GCP resource group:

```
terraform/
├── main.tf                     # Provider, API enablement, module wiring
├── variables.tf                # project_id, region, db_tier, etc.
├── outputs.tf                  # Deployed URLs and connection info
├── terraform.tfvars.example    # Template for real values
├── modules/
│   ├── networking/             # VPC, private subnet, serverless VPC connector
│   ├── database/               # Cloud SQL PostgreSQL 16 (private IP only)
│   ├── redis/                  # Memorystore Redis 7 (VPC-connected)
│   ├── kms/                    # KMS keyring + crypto key (90-day rotation)
│   ├── secrets/                # Secret Manager (DATABASE_URL, REDIS_URL)
│   ├── artifact-registry/      # Docker image repository (keeps last 10)
│   ├── cloud-run/              # ECS (public) + mock-hss (internal only)
│   ├── cloud-armor/            # WAF: rate limit, SQLi, XSS protection
│   └── iam/                    # 3 service accounts with least-privilege roles
```

The dependency graph flows bottom-up: `networking` → `database`/`redis` → `kms`/`secrets` → `cloud-run`. Terraform resolves this automatically from module references.

The most important security decision: **mock-hss has `INGRESS_TRAFFIC_INTERNAL_ONLY`**. It cannot be reached from the internet. Only the ECS Cloud Run service (which is inside the same VPC) can call it.

### CI/CD: Every Push Deploys

The `cloudbuild.yaml` defines a 13-step pipeline:

1. Install dependencies (ECS + mock-hss in parallel)
2. TypeScript compile (both)
3. Run unit tests
4. Build Docker images (tagged with commit SHA + `latest`)
5. Push images to Artifact Registry
6. Run database migrations (via Cloud SQL Proxy)
7. Deploy mock-hss to Cloud Run
8. Deploy ECS to Cloud Run
9. Smoke test: `curl /health`

The key ordering constraint: mock-hss deploys *before* ECS. The ECS Cloud Run service references mock-hss's URL (injected as the `HSS_URL` environment variable). If ECS deployed first and mock-hss was down, health checks would fail.

### The Migration Runner

Database migrations don't run at container startup. This avoids a nasty race condition: Cloud Run can spin up multiple instances simultaneously. If two instances both try to apply the same migration, you get a partial migration or a deadlock.

Instead, migrations run as a dedicated Cloud Build step (`src/db/migrate.ts`) *before* any containers are deployed. One migration run, then N instances can start safely.

### Lessons From Phase 8

#### 1. Terraform Modules as Boundaries of Responsibility

Each module has exactly three files: `main.tf` (resources), `variables.tf` (inputs), `outputs.tf` (outputs). A module doesn't reach into another module's state — it only uses declared outputs. This is the same principle as function parameters and return values: explicit interfaces make dependencies visible.

**Takeaway:** When your infrastructure has more than 5 resources, modularize it. The module boundaries should map to your mental model of the system ("the database," "the networking layer"), not to arbitrary file size limits.

#### 2. Secrets Belong in Secret Manager, Not Environment Variables

Our Terraform creates secrets in Secret Manager and injects them into Cloud Run as environment variables *from* Secret Manager. The `terraform.tfvars` file (which holds the GCP project ID, not secrets) is in `.gitignore`. The actual database password is generated by Terraform's `random_password` resource — no human ever sees or types it.

**Takeaway:** If a value is sensitive, it should flow from a secrets manager to the runtime environment automatically. Humans should never copy-paste passwords.

#### 3. Cloud Armor Rules Are Cheap Insurance

SQL injection and XSS protection rules are pre-configured in Google's WAF. Enabling them costs nothing extra and blocks common attack patterns. Rate limiting at 100 req/s per IP prevents abuse without affecting legitimate traffic.

**Takeaway:** WAF rules are almost always worth enabling. They're not a substitute for input validation in your code, but they catch attacks before your code even runs.

#### 4. Service Accounts as Identity Boundaries

We created three service accounts with distinct, minimal permissions:
- `ecs-runner`: Cloud SQL + Secret Manager + Cloud Trace (no KMS access)
- `mock-hss-runner`: Cloud SQL + Secret Manager + KMS decrypt + Cloud Trace
- `cloud-build-deployer`: Cloud Run admin + Artifact Registry + Secret Manager

The ECS cannot decrypt subscriber keys. The mock HSS cannot deploy new versions. The build system cannot decrypt subscriber keys. Each account can do exactly what it needs and nothing more.

**Takeaway:** Least-privilege isn't just a security checkbox. It's a design constraint that makes your system's trust boundaries explicit.

### Hardening the Cloud Run Module: 9 Lessons That Would Have Bitten Us in Production

After the initial Terraform was written, we did a hardening pass on the Cloud Run module. Every fix below addresses something that would have caused a real production incident — not a theoretical one.

#### 5. VPC Connectors Are Legacy — Use Direct VPC Egress

The original Terraform used a `google_vpc_access_connector` — a legacy approach that spins up hidden `e2-micro` proxy VMs between Cloud Run and the VPC. These add latency on every call to Postgres, Redis, and mock-hss, have throughput limits, and cost extra.

The fix: replace `vpc_access.connector` with `vpc_access.network_interfaces`, which places Cloud Run instances directly on the VPC subnet. No proxy, no extra VMs, no throughput bottleneck.

```hcl
# BEFORE (legacy — proxy VMs in the path)
vpc_access {
  connector = var.vpc_connector_id
  egress    = "PRIVATE_RANGES_ONLY"
}

# AFTER (direct — instances sit on the subnet)
vpc_access {
  network_interfaces {
    network    = var.vpc_network
    subnetwork = var.vpc_subnetwork
  }
  egress = "PRIVATE_RANGES_ONLY"
}
```

**Takeaway:** When GCP offers two ways to do something and one involves extra infrastructure, check if there's a newer, simpler approach. VPC Connectors were the only option before Direct VPC Egress existed — they're not wrong, just outdated.

#### 6. Cloud SQL Auth Proxy Needs an Explicit Volume Mount

Passing a `DATABASE_URL` with a private IP doesn't give you encrypted, authenticated connections to Cloud SQL. The Cloud SQL Auth Proxy handles mTLS and IAM authentication automatically — but only if you tell Cloud Run to start it.

The fix: add a `volumes` block with `cloud_sql_instance`, mount it at `/cloudsql/`, and change the `DATABASE_URL` to use the Unix socket path:

```hcl
# In the Cloud Run template
volumes {
  name = "cloudsql"
  cloud_sql_instance {
    instances = [var.cloudsql_connection_name]  # "project:region:instance"
  }
}

# In the container
volume_mounts {
  name       = "cloudsql"
  mount_path = "/cloudsql"
}
```

And the `DATABASE_URL` changes from:
```
postgresql://ecs:pass@10.0.0.5:5432/entitlements
```
to:
```
postgresql://ecs:pass@/entitlements?host=/cloudsql/project:region:instance
```

**Takeaway:** Cloud Run's Cloud SQL integration isn't automatic. You need three things working together: the volume declaration, the mount point, and a socket-based connection string. Miss any one and your app either can't connect or connects insecurely.

#### 7. `min_instance_count = 0` Is a Production Incident Waiting to Happen

The default scale-to-zero sounds great for cost savings, but EAP-AKA is a multi-round-trip protocol with chained dependencies. A fully cold request stacks up:

```
ECS cold start (container boot + Node.js init)    ~1-3s
  → Redis connection establishment                 ~100ms
  → Call mock-hss (also cold):
      mock-hss cold start                          ~1-3s
        → Postgres connection                      ~200ms
        → KMS API call                             ~100ms
───────────────────────────────────────────────────
Worst case total:                                  ~5-10s
```

Add mobile network latency and the device's HTTP timeout (often 30s) becomes reachable. Setting `min_instance_count = 1` on both services keeps at least one warm container always ready. The cost difference is minimal — idle instances are billed at a reduced CPU rate.

**Takeaway:** For anything latency-sensitive with chained service dependencies, scale-from-one, not scale-from-zero. The cost of one idle container is trivial compared to the cost of timing out real users.

#### 8. 512Mi Is a Ticking Time Bomb for Node.js

512MiB sounds like a lot until you add up: V8 engine, Fastify with compiled TypeBox schemas, Postgres connection pool, Redis client, OpenTelemetry instrumentation, and the actual application. Under burst traffic, V8's garbage collector needs headroom — without it, GC pauses spike (freezing all request handling) or the container OOM-crashes.

We bumped to 1Gi. Node.js defaults its heap limit to ~75% of available memory, giving ~768MB heap — enough for normal operation plus GC overhead.

**Takeaway:** Memory limits aren't about average usage — they're about peak usage during garbage collection. If your limit is close to your steady-state usage, GC will eventually push you over.

#### 9. CPU Throttling Silently Kills Connection Pools

This is the sneakiest gotcha. Cloud Run defaults to throttling CPU to zero between requests. That means:

1. A request completes, CPU drops to zero
2. Postgres and Redis connection pools can't send keep-alive packets
3. The server/firewall closes idle connections after ~30s
4. Next request arrives, pool tries to use dead connections → errors
5. Pool reconnects (adding latency), or the request fails entirely

The fix is `cpu_idle = false` in the container's `resources` block. But here's the meta-lesson...

#### 10. Use Native Terraform Fields, Not Annotations

Our first attempt used the `run.googleapis.com/cpu-throttling` annotation:

```hcl
# WRONG — Terraform may silently ignore this
annotations = {
  "run.googleapis.com/cpu-throttling" = "false"
}
```

This works in YAML manifests deployed via `gcloud`, but in Terraform's `google_cloud_run_v2_service` resource, there's a native `cpu_idle` field. When both exist, Terraform uses the native field and may ignore the annotation — meaning your annotation looks correct but has no effect.

```hcl
# RIGHT — native Terraform field, guaranteed to apply
resources {
  limits = {
    cpu    = "1"
    memory = "1Gi"
  }
  cpu_idle          = false  # Keep CPU allocated between requests
  startup_cpu_boost = true   # Extra CPU during cold start
}
```

**Takeaway:** Always check if the Terraform provider has a native field before using annotations. Annotations are a pass-through escape hatch — native fields are validated, documented, and won't be silently ignored.

#### 11. Internal Services Still Need Explicit Invoker Permissions

We had mock-hss set to `INGRESS_TRAFFIC_INTERNAL_ONLY` and assumed that meant any service in the same project could call it. Wrong. Cloud Run enforces IAM on every request, including internal service-to-service calls. Without `roles/run.invoker` on mock-hss granted to the ECS service account, every call returns 403.

```hcl
resource "google_cloud_run_v2_service_iam_member" "ecs_invokes_mock_hss" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.mock_hss.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${var.ecs_service_account_email}"
}
```

**Takeaway:** "Internal only" in Cloud Run means "only reachable from the VPC" — it doesn't mean "open to everything inside." IAM is always enforced. This is actually a feature: even if an attacker gets into your VPC, they can't call internal services without the right service account.

#### 12. Service Accounts Need Logging and Monitoring Roles

The initial IAM config had Cloud SQL and Cloud Trace roles but forgot `logging.logWriter` and `monitoring.metricWriter`. Without these:
- Pino's structured JSON logs would silently fail to appear in Cloud Logging
- OpenTelemetry metrics would fail to export to Cloud Monitoring

Both are **silent failures** — the application runs fine, but you're flying blind. You'd only discover it when you need the logs to debug a production issue and they're not there.

**Takeaway:** Always include observability roles (`logging.logWriter`, `monitoring.metricWriter`, `cloudtrace.agent`) in your service account setup. Test that logs and metrics actually appear — don't just assume.

#### 13. Enterprise GCP Org Policies Fight You at Every Turn

When we tried to grant IAM roles to deploy, we hit two enterprise-specific issues:

1. **`user:` bindings are blocked** — the org policy requires `group:` bindings. You can't grant roles to individual users; everything must go through Google Groups.
2. **Conditional IAM policies require `--condition=None`** — if the project already has any conditional IAM bindings, every new binding command requires you to explicitly specify `--condition=None` (even for unconditional bindings).

Neither error message is obvious. The first says "disallowed member type" (no hint about groups). The second says "specifying a condition is required" (sounds like you need a condition, when actually you need to explicitly say "no condition").

**Takeaway:** Enterprise GCP is a different beast from personal GCP. Org policies, conditional IAM, and group-only bindings are the norm. Always test your IAM commands early in the deployment process — don't save them for last.

---

## The Full Architecture (After Phase 8)

```
src/
├── index.ts                          # Entry point: OTel → dotenv → start server
├── config/
│   ├── index.ts                      # Typed config (port 8080, pool size 2, GCP_PROJECT_ID)
│   ├── constants.ts                  # 12 AppIDs, HTTP codes, EAP-AKA attributes
│   ├── logger.ts                     # Pino logger with Cloud Logging severity
│   ├── tracing.ts                    # OpenTelemetry (HTTP, pg, ioredis)
│   └── metrics.ts                    # OTel metrics provider
├── db/
│   ├── schema.ts                     # 5 tables
│   ├── index.ts                      # PostgreSQL pool + Drizzle ORM
│   ├── redis.ts                      # IORedis client (lazy connect)
│   ├── migrate.ts                    # Migration runner (Cloud Build step)
│   └── seed-entitlements.ts          # Idempotent seed (23 records)
├── auth/                             # EAP-AKA + token management
├── protocol/                         # Request schemas, response types, builders
├── services/                         # 12 service handlers
└── server/                           # Fastify app + routes + middleware

mock-hss/src/
├── config.ts                         # KMS env vars (LOCAL_KEK_HEX or GCP KMS)
├── kms.ts                            # LocalKeyManager + CloudKmsKeyManager
├── index.ts                          # Key manager selection via createKeyManager()
└── ...

terraform/                            # 10 modules: networking, database, redis, kms,
│                                     # secrets, artifact-registry, cloud-run,
│                                     # cloud-armor, iam, load-balancer
cloudbuild.yaml                       # 13-step CI/CD pipeline
```

---

## Deploying to GCP: What You Actually Need

All the Terraform modules and CI/CD pipeline are written, but deploying requires a few things from the real world.

### The One Thing You Must Have

A **GCP project with billing enabled.** Every resource — Cloud SQL, Memorystore, KMS, Cloud Run — lives inside a project. Without billing, none of them can be created. If you're experimenting, Google offers $300 in free credits for new accounts.

### Tools on Your Machine

You need three tools installed locally:

| Tool | Why | Install |
|------|-----|---------|
| **`gcloud` CLI** | Authenticates you to GCP and lets you manage resources from the terminal | `brew install google-cloud-sdk` or [cloud.google.com/sdk](https://cloud.google.com/sdk/docs/install) |
| **Terraform >= 1.5** | Reads our `terraform/` modules and provisions all the infrastructure | `brew install terraform` or [terraform.io](https://developer.hashicorp.com/terraform/install) |
| **Docker** | Builds the container images for the first manual push (CI/CD takes over after that) | [docker.com](https://docs.docker.com/get-docker/) |

### Authentication

Two commands get you authenticated:

```bash
gcloud auth login                        # Your browser opens, you log in
gcloud auth application-default login    # Creates credentials Terraform can use
```

Your account needs **Owner** or **Editor** role on the project. This is because Terraform needs to enable APIs, create service accounts, set IAM policies, and create resources across multiple GCP services.

### Decisions to Make (There Are Only 5)

| Variable | What it is | Default | Do you need to change it? |
|----------|-----------|---------|--------------------------|
| `project_id` | Your GCP project ID | *none — required* | Yes, always |
| `region` | Where to deploy | `us-central1` | Only if you need a specific region |
| `db_tier` | Cloud SQL machine size | `db-f1-micro` | No — cheapest tier, fine for POC |
| `db_ha_enabled` | Database high availability | `false` | No — unnecessary for POC |
| `operator_mcc` / `operator_mnc` / `operator_name` | Operator identity in TS.43 responses | `001` / `01` / `TestOperator` | Only if simulating a specific carrier |

Everything else is wired up automatically by the Terraform modules.

### The Deployment Sequence

Think of this as a five-act play. Each act depends on the one before it.

**Act 1: Configure** — Tell Terraform about your project.

```bash
cp terraform/terraform.tfvars.example terraform/terraform.tfvars
# Edit terraform.tfvars and fill in your project_id
```

**Act 2: Provision** — Terraform creates ~30 GCP resources in the right order.

```bash
cd terraform
terraform init    # Downloads the Google provider
terraform plan    # Shows exactly what will be created (review this!)
terraform apply   # Creates everything: VPC, Cloud SQL, Redis, KMS,
                  # Secret Manager, Artifact Registry, Cloud Run services,
                  # Cloud Armor WAF, 3 service accounts with least-privilege IAM
```

This takes 5–15 minutes. Cloud SQL is the slowest to provision.

**Act 3: Build & Push** — Build Docker images and push them to Artifact Registry.

```bash
# Authenticate Docker to Artifact Registry
gcloud auth configure-docker us-central1-docker.pkg.dev

# Build and push both images
docker build -t us-central1-docker.pkg.dev/YOUR_PROJECT/entitlements/ecs:latest .
docker build -t us-central1-docker.pkg.dev/YOUR_PROJECT/entitlements/mock-hss:latest mock-hss/
docker push us-central1-docker.pkg.dev/YOUR_PROJECT/entitlements/ecs:latest
docker push us-central1-docker.pkg.dev/YOUR_PROJECT/entitlements/mock-hss:latest
```

**Act 4: Database Setup** — Run migrations and seed test data.

```bash
npx tsx src/db/migrate.ts              # Apply schema migrations
npx tsx src/db/seed-entitlements.ts    # Seed Alice, Bob, Charlie + 23 entitlements
```

**Act 5: Verify** — Check that everything is alive.

```bash
# Get the deployed URL
ECS_URL=$(gcloud run services describe entitlement-server \
  --region=us-central1 --format='value(status.url)')

# Health check
curl $ECS_URL/health
# → {"status":"ok"}

# EAP-AKA challenge test
curl -s -X POST $ECS_URL/entitlement \
  -H 'Content-Type: application/json' \
  -d '{"app":"ap2004","terminal_id":"123456789012345","entitlement_version":"2","imsi":"001010000000001"}' \
  -D -
# → 401 + EAP-Challenge (authentication is working!)
```

### Wiring Up CI/CD (Optional, After First Deploy)

Once the first manual deploy works, you can connect GitHub to Cloud Build so that every push to `main` automatically builds, tests, and deploys:

1. Go to **Cloud Build > Triggers** in the GCP Console
2. Click **"Connect Repository"** and authorize your GitHub repo
3. Create a trigger pointing at `cloudbuild.yaml` on the `main` branch

After this, the 13-step pipeline in `cloudbuild.yaml` handles everything: install → compile → test → build images → push → migrate → deploy mock-hss → deploy ECS → smoke test.

### What Does It Cost?

For a POC that sits mostly idle:

| Resource | Monthly Cost |
|----------|-------------|
| Cloud SQL (`db-f1-micro`) | ~$8 |
| Memorystore Redis (1GB basic) | ~$35 |
| Cloud Run (both services) | Near-zero when idle |
| KMS (1 key, 90-day rotation) | ~$0.06 |
| Artifact Registry + Secret Manager | Negligible |
| **Total** | **~$40–50/month** |

Memorystore is the biggest line item by far. If you're just running quick tests and don't need a persistent Redis, you could skip it and use a Cloud Run-hosted Redis container instead (not recommended for production, but fine for a quick POC demo).

### What Terraform Creates (The Full Resource Map)

For the curious, here's everything that `terraform apply` provisions:

```
Networking          → VPC + subnet + VPC connector + private service access
Cloud SQL           → PostgreSQL 16 instance + database + user + generated password
Memorystore         → Redis 7 instance (1GB, BASIC tier, VPC-connected)
KMS                 → Keyring "entitlement-keys" + CryptoKey "ki-kek" (90-day rotation)
Secret Manager      → database-url + redis-url secrets (auto-populated from other modules)
Artifact Registry   → Docker repo "entitlements" (cleanup policy: keep last 10 images)
Cloud Run (ECS)     → Internal+GCLB ingress, port 8080, Direct VPC Egress, secrets injected
Cloud Run (mock-hss)→ Internal-only service, port 3001, Direct VPC Egress, KMS access
Cloud Armor         → WAF policy: rate limit (2000 req/5min/IP), SQLi + XSS protection
Load Balancer       → Global External ALB, static IP, serverless NEG, SSL termination,
                      HTTP→HTTPS redirect, Cloud Armor attached
IAM                 → 3 service accounts: ecs-runner, mock-hss-runner, cloud-build-deployer
GCP APIs            → 11 APIs enabled (Cloud Run, SQL, Redis, KMS, etc.)
```

The dependency graph flows bottom-up: networking first, then database/redis, then KMS/secrets, then Cloud Run services on top. Terraform figures out the order automatically from module references.

---

## Phase 8b: External Application Load Balancer — The Front Door

### Why an ALB?

Up until now, Cloud Run's ECS service was directly exposed to the internet with `INGRESS_TRAFFIC_ALL` and an `allUsers` IAM binding. That works, but it means Cloud Armor's WAF rules can't actually do much — they need a Google Load Balancer in front to intercept and filter traffic. It's like having a bouncer on your payroll but letting people walk in through the back door.

The fix: put a **Global External Application Load Balancer** in front of Cloud Run, then lock down Cloud Run to only accept traffic from the load balancer.

### The Architecture After the ALB

```
Internet → Static IP:443 → HTTPS Proxy (TLS termination)
                              → URL Map → Backend Service (Cloud Armor attached)
                                            → Serverless NEG → Cloud Run ECS
                                                                (internal + GCLB only)

Internet → Static IP:80  → HTTP Proxy → 301 Redirect → :443
```

Every request from the internet hits the ALB first. The ALB terminates TLS, applies Cloud Armor WAF rules (rate limiting, SQLi/XSS blocking), and then forwards clean traffic to Cloud Run over Google's internal network. Cloud Run rejects anything that doesn't come through the ALB.

### Two Layers of Lockdown

We didn't just change the ingress setting — we removed two things:

1. **`INGRESS_TRAFFIC_ALL` → `INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER`** — Cloud Run now rejects requests that don't come from within the VPC or from Google's load balancer infrastructure. Someone curling the Cloud Run URL directly gets blocked at the network level.

2. **Removed the `allUsers` IAM binding** — Even if someone bypassed the ingress filter, they'd need IAM authorization to invoke the service. Without `allUsers`, unauthenticated requests get a 403.

Two independent layers. An attacker would need to defeat both to reach the service directly.

### The SSL Certificate Problem (And a Dev-Friendly Solution)

A load balancer that terminates TLS needs a certificate. For production with a real domain, Google manages the cert automatically — provisioning, renewal, the works. But what about dev/staging without a domain, where you're hitting a raw IP address?

A Google-managed cert requires a domain for DNS validation. No domain means no cert. And an HTTPS proxy with an empty `ssl_certificates` list simply doesn't work — it won't serve TLS at all. Port 443 becomes a black hole.

The solution: when no domain is configured, Terraform generates a **self-signed certificate** using the `tls` provider:

```hcl
resource "tls_private_key" "self_signed" {
  count     = var.domain == null ? 1 : 0
  algorithm = "RSA"
  rsa_bits  = 2048
}

resource "tls_self_signed_cert" "self_signed" {
  count           = var.domain == null ? 1 : 0
  private_key_pem = tls_private_key.self_signed[0].private_key_pem
  validity_period_hours = 8760  # 1 year

  subject {
    common_name  = "entitlement-server.dev.internal"
    organization = "Development"
  }

  allowed_uses = ["key_encipherment", "digital_signature", "server_auth"]
}
```

Browsers will warn about the untrusted cert, but `curl -k` and development tools work fine. When you're ready for production, set `domain = "ecs.yourcarrier.com"` and Terraform swaps to a Google-managed cert automatically.

The `create_before_destroy` lifecycle on the self-managed cert is important — GCP cert names are immutable, so rotation requires creating the new cert, attaching it to the proxy, and then deleting the old one. Without this lifecycle rule, Terraform tries to delete first and fails because the cert is still in use.

### The HTTP→HTTPS Redirect

Port 80 gets its own forwarding rule, URL map, and HTTP proxy — but instead of routing to a backend, it returns a `301 Moved Permanently` redirect to the HTTPS URL. This is standard practice: never serve real traffic over HTTP, but don't let port 80 be a dead end either.

### The `EXTERNAL_MANAGED` Scheme

All forwarding rules and the backend service use `load_balancing_scheme = "EXTERNAL_MANAGED"` instead of the older `EXTERNAL`. The "managed" variant is Google's modern ALB implementation — it supports Cloud Armor integration, advanced traffic management, and is the only option that works with serverless NEGs. Using plain `EXTERNAL` with a serverless NEG silently produces a broken configuration.

### Lessons From the ALB

#### 14. GCP's Terraform Provider Has Its Own Vocabulary

The GCP Console and `gcloud` CLI call it `INTERNAL_AND_GCLB`. The Terraform provider calls it `INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER`. They mean the same thing, but if you copy the value from GCP docs into your Terraform config, `terraform validate` rejects it. This is a recurring pattern: **the Terraform provider's enum values often don't match the GCP documentation or Console UI.** Always check `terraform validate` early, and when something fails, look at the provider source code or error message for the correct enum.

#### 15. An HTTPS Proxy Without a Certificate Is a Silent Failure

There's no Terraform validation error for an HTTPS proxy with `ssl_certificates = []`. It provisions successfully. But port 443 simply doesn't work — connections hang or reset. The error only surfaces when you try to actually use the load balancer and wonder why nothing responds.

**Takeaway:** When a resource "works" in Terraform but doesn't function at runtime, check for empty lists or null references in its configuration. Terraform validates syntax, not semantics.

#### 16. Self-Signed Certs Need `create_before_destroy`

GCP certificate names are immutable. If Terraform needs to recreate a self-managed cert (e.g., the private key changed, or you renamed it), the default behavior is delete-then-create. But you can't delete a cert that's attached to a live HTTPS proxy — GCP returns an error. `create_before_destroy = true` + `name_prefix` (instead of a static `name`) solves this: Terraform creates the new cert with a unique suffix, re-points the proxy, then deletes the old one.

**Takeaway:** Any GCP resource that's referenced by another resource (certs → proxies, NEGs → backends, addresses → forwarding rules) should use `create_before_destroy` if it might ever be recreated.

### The Terraform Module

```
terraform/modules/load-balancer/
├── main.tf           # 10 resources: static IP, serverless NEG, backend service,
│                     # URL map, SSL cert (managed or self-signed), HTTPS proxy,
│                     # HTTPS forwarding rule, HTTP redirect URL map, HTTP proxy,
│                     # HTTP redirect forwarding rule
├── variables.tf      # project_id, region, cloud_run_service_name,
│                     # security_policy_id, domain (optional)
└── outputs.tf        # external_ip, https_url
```

The module is wired in `terraform/main.tf` and depends on both `cloud_run` (for the service name) and `cloud_armor` (for the WAF policy ID).

---

## What's Coming Next

- **Phase 9: Observability**: Custom application metrics, Cloud Monitoring dashboards, alerting policies, trace-log correlation
- **Phase 10: Testing & Hardening**: Load testing, error handling audit, logging completeness

---

## Quick Start

```bash
npm install                    # Install dependencies
npm run build                  # Compile TypeScript → dist/
docker compose up --build      # Start Postgres + Redis + Mock HSS + ECS
# Seed test subscribers (run once):
docker compose exec mock-hss npx tsx src/seed.ts

# Test the EAP-AKA challenge (Round Trip 1):
curl -s -X POST http://localhost:8080/entitlement \
  -H 'Content-Type: application/json' \
  -d '{"app":"ap2004","terminal_id":"12345678901234","entitlement_version":"2","imsi":"001010000000001"}' \
  -D -
# → 401 + eap_relay (base64 EAP-Challenge) + X-EAP-Session-Id header
```
