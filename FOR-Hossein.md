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

### The MILENAGE Algorithm

The cryptographic math inside the SIM card follows an algorithm called **MILENAGE** (3GPP TS 35.206). We implement it ourselves in TypeScript rather than using an existing npm package, for a specific reason: the only existing package has 7 GitHub stars, no TypeScript types, no security audit, and is unmaintained. When you're implementing something that protects subscriber identity, "it works, probably" isn't good enough.

The good news is that MILENAGE is elegant in its simplicity. The entire algorithm is built on a single cryptographic primitive: **AES-128 encryption**. Everything else is XOR operations and byte shuffling. We delegate AES to Node.js's `crypto` module (which calls OpenSSL, which uses hardware acceleration), so we never write our own crypto. We just wire up the specific sequence of operations the spec defines.

The even better news: the 3GPP published official test vectors — known inputs with expected outputs. If our implementation produces the exact same outputs for every test set, it's provably correct. There's no ambiguity.

---

## The Technology Stack (And Why Each Choice Was Made)

### Fastify Over Express

Express is the default choice for Node.js HTTP servers, but we picked **Fastify** for specific reasons that matter for this project:

1. **Schema-based validation**: TS.43 defines precise parameter sets for every request. Fastify validates incoming JSON against compiled schemas *before* our code runs. A malformed IMEI or invalid AppID never reaches the handler.

2. **Compiled serialization**: Fastify compiles JSON schemas into fast serializers at startup. Since every response is a structured entitlement config, this matters.

3. **Plugin encapsulation**: Each of the 13 service handlers (VoWiFi, VoLTE, ODSA, etc.) maps naturally to a Fastify plugin with its own routes and hooks.

4. **Performance**: ~2-3x higher throughput than Express. During a mass re-authentication event (say, after a network outage when a million devices all wake up at once), lower per-request overhead is a real advantage.

### TypeBox Over Zod

Both define validation schemas in TypeScript. We picked **TypeBox** because it outputs native JSON Schema, which Fastify compiles into Ajv validators at startup. Zod is a runtime interpreter — it re-evaluates schemas on every single request. Using Zod with Fastify would be like buying a sports car and then towing it with a truck. TypeBox also gives us a single source of truth: `Static<typeof schema>` infers TypeScript types from the validation schema, so they can never drift apart.

### PostgreSQL + Redis (Not One or the Other)

Different data has different lifetimes and access patterns:

- **Subscriber records, entitlements, and tokens** live in **PostgreSQL**. They need durability (survive restarts), relational integrity (tokens belong to subscribers), and auditability.

- **EAP-AKA sessions** live in **Redis only**. They last ~90 seconds, need sub-millisecond lookup, and losing one just means the device re-authenticates. Writing these to Postgres would be like using a filing cabinet for Post-it notes.

- **Auth tokens** use **both**: PostgreSQL is the durable source of truth; Redis is a fast-path cache. This "write-through cache" pattern means token validation (the hottest code path — every single request hits it) runs in sub-millisecond time, but tokens survive a Redis restart.

### Drizzle ORM

Lightweight, type-safe, and stays close to SQL. For a system where the database schema is well-defined and queries are straightforward, heavy ORMs like Prisma or TypeORM add complexity without benefit.

---

## Security Architecture: Thinking Like a Paranoid Engineer

### Why the Mock HSS Is a Separate Service

This was one of the most important architectural decisions, and it came from asking: *"What's the worst thing that could happen?"*

The ECS is internet-facing. It's the service that attackers can reach. If it's compromised, what can the attacker get?

If Ki (the subscriber secret key) lives inside the ECS, the answer is: *everything*. Every subscriber's identity. The ability to impersonate any device on the network.

So we made a rule: **the ECS never touches Ki.** Not encrypted Ki, not the KMS key to decrypt Ki, nothing. The ECS calls the Mock HSS over the internal VPC and gets back derived vectors (RAND, AUTN, XRES, CK, IK). It has no way to reconstruct Ki from these — that's a mathematical property of the MILENAGE algorithm.

The Mock HSS has no internet access. To get Ki, you'd need to compromise:
1. The ECS (to reach the internal network)
2. The Mock HSS (a separate service with its own attack surface)
3. Cloud KMS (to unwrap the encryption keys)
4. The database (to get the encrypted Ki)

All four, simultaneously. That's defense in depth.

### Envelope Encryption

Ki is encrypted at rest using **envelope encryption**: each subscriber has their own Data Encryption Key (DEK), and all DEKs are wrapped by a Key Encryption Key (KEK) that lives in Google Cloud KMS hardware. The KEK never leaves the KMS boundary. If someone steals the database, they get ciphertext. If someone compromises KMS alone, they don't have the per-subscriber wrapped DEKs. You need both.

### EAP-AKA Response Idempotency

Here's a subtle bug that took careful thought to prevent: what if the network drops the server's 200 OK response after authentication succeeds?

The device retries. But the server already consumed the session, issued the token, and transitioned to SUCCESS state. Without special handling, the device either gets forced through a full re-authentication (wasteful and slow) or gets an error (wrong).

The fix: on successful authentication, we cache the complete HTTP response in Redis, keyed by a fingerprint of the request. If the same request arrives again within 90 seconds, we replay the cached response verbatim. The device gets the token it earned. After 90 seconds, the cache expires and a fresh authentication starts (which is correct behavior by then).

---

## GCP Architecture: Production-Grade Infrastructure

### Cloud Run: The Right Fit

Cloud Run runs containers that auto-scale based on traffic. Our ECS is a stateless HTTP server — all state lives in Cloud SQL and Memorystore. This is a perfect match: no cluster management (unlike GKE), auto-scaling from 1 to 100+ instances in seconds, and pay-per-request pricing.

### The Cold Start Problem

Cloud Run scales to zero by default. That's great for cost, terrible for EAP-AKA. The authentication handshake requires 2+ HTTP round-trips, and if both the ECS and Mock HSS are cold, the startup chain stacks up to 3-6 seconds before the first response. Add mobile network latency, and you risk timeouts.

The fix: `minInstances: 1` on both services. At least one container is always warm with live database and Redis connections. Cost is minimal (Cloud Run bills idle instances at reduced rates) vs. the risk of first-request failures.

### Connection Pool Management

Here's a problem specific to serverless: Cloud Run can scale from 1 to 100 instances in seconds during a traffic spike. Each instance opens a database connection pool. If each pool has 10 connections (the library default), 100 instances means 1,000 connections — far more than Cloud SQL supports.

We address this with three layers:
1. **Small pool per instance**: `DB_POOL_SIZE=5` (configurable at deployment time)
2. **Instance ceiling**: `maxInstances: 100` (hard cap: 100 x 5 = 500 max connections)
3. **Managed connection pooling**: For production scale, Cloud SQL Enterprise Plus offers a built-in PgBouncer-compatible pooler. Switching is a `DATABASE_URL` port change — no code changes.

The key insight: both `DB_POOL_SIZE` and `maxInstances` are deployment-time parameters, not hardcoded. Operators tune them for their Cloud SQL tier.

### Cloud Armor: DDoS and WAF at the Edge

The ECS is internet-facing and will receive traffic from millions of devices. Cloud Armor sits in front of the load balancer and inspects all traffic before it reaches our application:

- **Volumetric DDoS** is absorbed at Google's edge network (SYN floods, UDP reflection)
- **Rate limiting** throttles per-IP to 100 req/min (devices authenticate infrequently — sustained high rates mean abuse)
- **Geo-restriction** blocks countries with no subscriber base
- **OWASP rules** catch SQL injection, XSS, and protocol attacks (defense-in-depth with TypeBox validation)
- **Request size limits** reject bodies over 8KB (entitlement requests are small)

The critical point: attacks are handled *before* reaching Cloud Run. This prevents both service disruption *and* cost spikes from auto-scaling to absorb junk traffic.

### Direct VPC Egress (Not VPC Connectors)

VPC Connectors are the older way to connect Cloud Run to a VPC. They provision a group of small VMs that act as proxies — adding latency, throughput limits, and cost. Direct VPC Egress places Cloud Run instances directly on the VPC subnet with no intermediary. Lower latency to Redis and Cloud SQL, no connector to manage, no extra billing.

---

## Observability: Seeing What's Happening

A system you can't observe is a system you can't operate. We built observability in from day one, not as an afterthought.

### The Three Pillars

1. **Cloud Logging** (structured logs via Pino): Every request logs app_id, operation, response code, and latency. Every EAP-AKA step logs session state transitions. Trace IDs are embedded in every log line so you can click a trace and see all correlated logs.

2. **Cloud Trace** (distributed tracing via OpenTelemetry): A single request touches ECS → Redis → Mock HSS → Postgres → KMS. Traces show exactly where time is spent. Auto-instrumentation patches `http`, `pg`, and `ioredis` transparently — no manual span creation needed for most paths.

3. **Cloud Monitoring** (metrics and alerting): 11 custom metrics covering auth success rates, token cache hit ratios, HSS latency, DB pool utilization, and more. Alerting policies fire before users notice degradation.

### What We Never Log

This is as important as what we do log: **Ki, OP, DEK, wrapped DEK, token values, and EAP-Response content are never logged at any level.** Tokens are redacted to their last 8 characters. A log leak should never become a key leak.

---

## The Codebase: How It's Organized

```
src/
  config/           ← Environment, logging, tracing, metrics (initialized first)
  server/           ← Fastify app, routes, middleware hooks
  auth/             ← EAP-AKA state machine, codec, token management
  services/         ← 13 entitlement handlers (one per AppID)
  protocol/         ← TypeBox schemas, XML/JSON response builders
  db/               ← Drizzle schema, migrations, data access
  mock/             ← Test data, mock BSS/SM-DP+
  utils/            ← Crypto helpers, validation utilities

mock-hss/           ← SEPARATE SERVICE (its own Dockerfile, package.json)
  src/
    milenage.ts     ← MILENAGE algorithm (AES-128 via Node crypto)
    kms.ts          ← Envelope encryption (Cloud KMS / local KEK)
    db.ts           ← Read encrypted Ki from Postgres

tests/
  unit/             ← Codec, crypto, MILENAGE test vectors
  integration/      ← Full EAP-AKA handshake, entitlement flows
  fixtures/         ← Sample requests/responses, AKA test vectors
```

The key structural decision: `mock-hss/` is a completely separate application with its own Dockerfile. It shares the Postgres database but has its own connection pool, its own Cloud Run service, and its own IAM permissions. From the ECS's perspective, it's just a URL (`HSS_URL`) that returns AKA vectors.

---

## Lessons and Takeaways

### 1. Separate Your Trust Boundaries

The most impactful architectural decision was making the Mock HSS a separate service. The initial instinct was to embed it in the ECS — it's simpler, it's one fewer service to deploy, and "it's just a POC." But that would mean the internet-facing server holds the keys to impersonate any subscriber. Good engineers ask: *"What happens when this gets compromised?"* — not *if*, but *when*.

**The lesson:** When you have data with different sensitivity levels (public HTTP requests vs. subscriber secret keys), put them in different processes with different network access and different IAM permissions. The boundary should match the threat model.

### 2. Don't Use Third-Party Libraries for Security-Critical Crypto (Unless They're Audited)

The existing MILENAGE npm package would have been the quick path. But it has 7 stars, no TypeScript types, no security audit, and no recent maintenance. For something that protects subscriber identity, "it seems to work" isn't a standard. We wrote our own, delegated the actual crypto primitive (AES-128) to Node.js's battle-tested `crypto` module, and validated against the 3GPP's official test vectors.

**The lesson:** The build-vs-buy decision for cryptographic code depends on the maturity of the available library. A well-audited, widely-used library (like `crypto`, OpenSSL, or `libsodium`) is almost always better than rolling your own. But an obscure, unmaintained library with no audit is worse than a careful implementation validated against a spec's official test vectors.

### 3. Idempotency Isn't Optional for Multi-Step Protocols

EAP-AKA requires 2+ HTTP round-trips. Networks drop packets. If you don't handle retries of the final step, you force devices through unnecessary re-authentication or return errors for already-authenticated sessions. The cached response replay pattern (fingerprint → Redis → replay) is simple but essential.

**The lesson:** Any multi-step protocol over an unreliable network needs an idempotency strategy. Ask yourself: *"What happens if the client never received my response and retries?"* Design for it before you write the handler code.

### 4. Serverless Doesn't Mean Stateless Concerns Disappear

Cloud Run auto-scales beautifully, but it creates new problems:
- **Connection exhaustion**: 100 instances x 10 default pool size = 1,000 connections. Cloud SQL can't handle that.
- **Cold starts**: A multi-service chain (ECS → Mock HSS) with cold starts on both ends can stack up to 6 seconds.
- **Cost amplification from attacks**: Without Cloud Armor, a DDoS causes Cloud Run to auto-scale to absorb junk traffic — and you pay for every instance.

**The lesson:** Serverless trades operational complexity for architectural complexity. You get auto-scaling for free, but you need to think about connection budgets, cold start chains, and cost protection. Configure `maxInstances`, `minInstances`, and pool sizes as deployment-time parameters — not hardcoded values.

### 5. Validation Belongs at the Framework Level, Not in Your Code

Hand-writing request validation in every handler is tedious, error-prone, and inconsistent. By using TypeBox schemas that Fastify compiles into Ajv validators at startup, we get:
- **Runtime protection** against malformed payloads (before handler code runs)
- **TypeScript types** inferred from the same schema (no duplicate definitions)
- **Consistent error responses** (Fastify returns structured 400 errors automatically)
- **Defense against injection** (regex patterns on IMEI, IMSI, MSISDN restrict to digits-only)

**The lesson:** Pick a framework that makes the secure path the easy path. If validation requires extra effort, developers will skip it. If it's baked into the route definition, it happens by default.

### 6. Observability Is Day-One Architecture, Not a Phase-2 Afterthought

We initialize OpenTelemetry tracing and metrics *before* Fastify starts. Auto-instrumentation patches `http`, `pg`, and `ioredis` at import time, so every database query and Redis command generates a trace span with no code changes. Trace IDs are embedded in every log line, so you can go from a Cloud Trace waterfall to the exact log entries for that request.

**The lesson:** If you add observability after the system is built, you end up with gaps — the one service call that isn't traced, the one error path that doesn't log. Wire it in at the foundation layer and let auto-instrumentation handle the rest.

### 7. Design for the Migration You Know Is Coming

The ECS doesn't know whether it's talking to a mock HSS or a real one. It calls `HSS_URL` and gets vectors back. Migrating to a real HSS means:
1. Set `HSS_URL` to the Diameter-to-HTTP gateway
2. Ensure the gateway returns the same JSON contract
3. No code changes

Similarly, switching from direct Cloud SQL connections to managed connection pooling is a `DATABASE_URL` port change (5432 → 6432). No code changes.

**The lesson:** Identify the integration points that will change in production and make them configuration-driven from the start. An interface that's easy to swap is worth more than an implementation that's optimized for today's deployment.
