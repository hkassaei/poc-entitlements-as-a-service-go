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

## What's Coming Next

This Phase 1 is the skeleton. The real meat comes in later phases:

- **Phase 2**: Mock HSS with MILENAGE cryptography (the SIM card math)
- **Phase 3**: EAP-AKA authentication flow (challenge-response over HTTP)
- **Phase 4**: Token management and entitlement queries
- **Phase 5**: Full service handlers for all 12 AppIDs
- **Phase 6**: Deployment to Google Cloud Run

Right now, if you `curl` the entitlement endpoint, you get a 501. By Phase 4, you'll get a real cryptographic authentication challenge back.

---

## Quick Start

```bash
npm install                    # Install dependencies
npm run build                  # Compile TypeScript → dist/
docker compose up              # Start Postgres + Redis + ECS
curl localhost:8443/health     # → { "status": "ok" }
```
