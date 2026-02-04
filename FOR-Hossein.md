# FOR-Hossein: Entitlements-as-a-Service, Explained

## What Is This Project?

Imagine you just bought a new phone and popped in your SIM card. You open the dialer and try to make a Wi-Fi call. Before the call connects, your phone silently asks your carrier: *"Is this person allowed to use Wi-Fi Calling?"* The carrier's server checks your subscription, checks your device, and sends back either a green light with configuration details, or a polite refusal.

That silent conversation is what this project implements. It's called an **Entitlement Configuration Server (ECS)** — the carrier-side system that answers the question *"What is this device allowed to do?"* for up to 13 different telecom services: Wi-Fi Calling, VoLTE, SMS over IP, eSIM activation, data plans, carrier billing, satellite connectivity, and more.

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
         |     |  13 service handlers     |           |
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

3. **Plugin encapsulation**: Each of the 13 service handlers (VoWiFi, VoLTE, ODSA, etc.) maps naturally to a Fastify plugin with its own routes and hooks.

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

## What's Coming Next

- **Deployment**: Google Cloud Run with managed TLS, Cloud Armor DDoS protection, and VPC Service Controls
- **More ODSA operations**: Server-initiated ODSA (ap2011), data plan info (ap2010)

Now if you `curl` the entitlement endpoint with an ODSA operation, you get real eSIM activation codes back!

---

## Quick Start

```bash
npm install                    # Install dependencies
npm run build                  # Compile TypeScript → dist/
docker compose up --build      # Start Postgres + Redis + Mock HSS + ECS
# Seed test subscribers (run once):
docker compose exec mock-hss npx tsx src/seed.ts

# Test the EAP-AKA challenge (Round Trip 1):
curl -s -X POST http://localhost:8443/entitlement \
  -H 'Content-Type: application/json' \
  -d '{"app":"ap2004","terminal_id":"12345678901234","entitlement_version":"2","imsi":"001010000000001"}' \
  -D -
# → 401 + eap_relay (base64 EAP-Challenge) + X-EAP-Session-Id header
```
