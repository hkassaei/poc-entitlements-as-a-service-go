# FOR-Hossein-Go.md

## The Go Re-implementation: Same Telecom Server, Completely Different Engine

Imagine you have a car that works perfectly well -- it gets you where you need to go, it's comfortable, it handles well. But it runs on premium gasoline, needs a minute to warm up every morning, and the engine is heavy enough that you can only carry a few passengers at a time. Now imagine someone offers you the same car body, same steering feel, same dashboard layout, but with an electric motor that starts instantly, weighs half as much, and can carry six times more passengers simultaneously. That's this project: the same Entitlements-as-a-Service (GSMA TS.43 compliant ECS) server, re-implemented from TypeScript to Go.

The original TypeScript version was roughly 3,300 lines of code. The Go version is about 7,500 lines across 58 files. More code, yes -- but Go is more explicit by nature. TypeScript lets you write `user?.name ?? "anon"` in one line; Go makes you write the if-else out loud. That verbosity is the trade. What you get in return is a server that handles ~500 concurrent requests per instance instead of ~80, uses ~256MB instead of ~1GB, and cold-starts in ~100ms instead of 2-3 seconds (no V8 JIT warmup, no node_modules to load, no TypeBox schemas to compile).

---

## What This Server Actually Does

Before diving into code structure, let's ground ourselves in what this thing is for.

When you turn on Wi-Fi Calling on your phone, or activate VoLTE, or pair a smartwatch as a companion device, your phone talks to an **Entitlement Configuration Server (ECS)**. The ECS is the gatekeeper: it verifies that you are who you say you are (using cryptographic proof from your SIM card), and then tells you what services you're entitled to.

The authentication protocol is **EAP-AKA** (Extensible Authentication Protocol - Authentication and Key Agreement), defined in RFC 4187. Think of it like a challenge-response handshake: the network says "prove you have this SIM card" by sending a random number, the SIM does some cryptographic math with its secret key, and sends the result back. If it matches what the network expected, you're in.

This is a multi-round-trip protocol:

1. **RT1**: Phone sends its IMSI (subscriber identity) -> Server generates a cryptographic challenge
2. **RT2**: Phone solves the challenge -> Server verifies the solution and grants entitlements

There's also a **fast re-authentication** path (RFC 4187 Section 5.1) that lets previously-authenticated devices skip the heavy crypto and re-authenticate with a counter-based mechanism. Think of it as the difference between going through airport security with a full check versus flashing your TSA PreCheck and walking through.

---

## Project Structure: Where Everything Lives

```
cmd/
  ecs/main.go                   # The main ECS server entry point
  mock-hss/main.go              # The mock Home Subscriber Server (simulates the telecom network)

internal/
  config/
    config.go                   # Environment-based configuration loading
    constants.go                # EAP-AKA constants (RFC 4187), App IDs (TS.43), states

  crypto/
    milenage.go                 # MILENAGE algorithm (3GPP TS 35.206) -- the SIM card math
    envelope.go                 # AES-256-GCM encryption, DEK wrap/unwrap
    kms.go                      # Key manager interface (local dev XOR vs Cloud KMS)

  eapaka/
    codec.go                    # EAP packet binary codec (TLV encode/decode)
    keys.go                     # Key derivation: MK, K_aut, K_encr, MSK, EMSK, FIPS 186-2 PRF
    encryption.go               # AES-128-CBC encryption for EAP inner attributes
    orchestrator.go             # Full auth flow (RT1 challenge, RT2 verify)
    reauth.go                   # Fast re-auth flow (RT1 challenge, RT2 verify)
    reauth_store.go             # Long-lived re-auth state (Redis, 48hr TTL)
    reauth_session.go           # Short-lived re-auth session (Redis, 90s TTL)
    session.go                  # Full-auth EAP session (Redis, 90s TTL)
    vectors.go                  # HSS client (fetches auth vectors over HTTP)
    idempotency.go              # SHA-256-keyed idempotency cache for EAP responses

  db/
    postgres.go                 # pgx/v5 connection pool
    redis.go                    # go-redis/v9 client
    queries.go                  # 10 raw SQL queries (no ORM)
    models.go                   # Struct definitions for 5 tables
    migrate.go                  # Schema migration runner
    seed.go                     # Development data seeding

  token/
    service.go                  # Token generation/validation with Redis write-through cache

  protocol/
    response.go                 # Internal response types (ApplicationConfig, ServiceEntitlementResponse)
    json_builder.go             # JSON response formatter
    xml_builder.go              # WAP-Provisioning XML response formatter (manual string building)
    statuscodes.go              # TS.43 status code constants
    request.go                  # Request type definitions
    apptypes.go                 # Application type helpers

  services/
    builder.go                  # The central switch: routes app IDs to service handlers
    vowifi.go                   # Wi-Fi Calling (ap2004)
    volte.go                    # VoLTE (ap2003)
    smsoip.go                   # SMS over IP (ap2005)
    odsa_companion.go           # ODSA Companion Device (ap2006)
    odsa_primary.go             # ODSA Primary Device (ap2009)
    odsa_common.go              # Shared ODSA logic
    data_plan.go                # Data Plan Information (ap2010)
    server_odsa.go              # Server-initiated ODSA (ap2011)
    dcb.go                      # Direct Carrier Billing (ap2012)
    private_identity.go         # Private User Identity (ap2013)
    device_user_info.go         # Device/User Information (ap2014)
    app_auth.go                 # Application Authentication (ap2015)
    sat_mode.go                 # Satellite Mode (ap2016)
    mock_smdp.go                # Mock SM-DP+ for eSIM provisioning

  server/
    server.go                   # chi router setup, HTTP server with timeouts
    routes.go                   # EntitlementHandler: POST/GET /entitlement with 4-path routing
    middleware.go               # UA parser, version check, recovery, logging
    helpers.go                  # JSON/XML response writers
    context.go                  # Context key helpers for request-scoped values
    audit.go                    # Audit logging middleware

sql/schema.sql                  # DDL for 5 tables (subscribers, devices, entitlements, tokens, audit_log)
Dockerfile.ecs                  # Multi-stage Go build -> distroless
Dockerfile.mock-hss
docker-compose.go.yml
Makefile                        # build, test, lint, vet, fmt, docker commands
```

The structure follows Go's idiomatic `internal/` convention. Everything under `internal/` is only importable by code inside this module -- Go enforces this at the compiler level. No other project can reach in and use our EAP codec or MILENAGE implementation. This is a nice security property that TypeScript simply doesn't have.

---

## Technology Choices and WHY

Every technology choice here was deliberate. Let me walk through each one and explain the reasoning, because understanding *why* is more valuable than knowing *what*.

### chi router (not Gin, not Echo, not stdlib alone)

The Go standard library's `net/http` is excellent, but it doesn't have route parameters or middleware chaining. Gin and Echo add a lot of framework machinery that we don't need. chi sits in the sweet spot: it's a thin layer on top of `net/http` that adds middleware chaining via `r.Use(...)` and clean route definitions, without reimagining the entire HTTP model. If you read chi middleware, it's just `func(http.Handler) http.Handler` -- the same interface the stdlib uses.

This matters because it maps cleanly from the TypeScript version's Express/Fastify middleware. In TypeScript we had:

```typescript
app.addHook('preHandler', userAgentParser);
```

In Go with chi, we have:

```go
r.Use(UserAgentParser)
```

Same concept, same ordering guarantees, but now each middleware is just a function that wraps an `http.Handler`. No framework magic.

### pgx/v5 raw SQL (not GORM, not sqlc, not ent)

This was probably the most opinionated choice. GORM is the "obvious" pick for Go database access, the way Drizzle or Prisma would be in TypeScript. But here's the thing: we have exactly 10 queries. They're fixed. They don't change at runtime. Several of them deal with `BYTEA` columns (encrypted keys) and `JSONB` columns (config data), which ORMs handle through layers of reflection and type coercion.

With raw SQL, every query is visible in one file (`queries.go`), and you can see exactly what SQL is being executed. There's no "what did the ORM generate?" guessing game. At 2000 RPS, the absence of reflection overhead is measurable. And when something goes wrong in production, you copy the SQL string directly into `psql` and debug it -- no ORM query-builder translation layer to reverse-engineer.

This is a lesson in engineering judgment: ORMs are great when you have dozens of dynamic queries, model relationships, and a team that isn't comfortable with SQL. When you have 10 fixed queries with heavy binary/JSON columns and a priority on performance and debuggability, raw SQL wins.

### go-redis/v9

Mature, well-maintained, supports Redis hashes (`HSET`/`HGETALL`), pipelines, and key expiry. All three of those features are critical here -- EAP sessions are stored as Redis hashes with 90-second TTL, and we use pipelines to batch `HSET` + `EXPIRE` into a single round trip.

### log/slog (stdlib)

Go 1.21 introduced `slog`, a structured logging package in the standard library. Zero external dependencies, outputs JSON that maps directly to Google Cloud Logging's expected format. In TypeScript, we used Pino (an external package). Here, we get the same structured JSON output with nothing to install:

```go
slog.Info("EAP-AKA challenge sent", "imsi", imsi, "sessionId", sessionID)
```

This outputs: `{"level":"INFO","msg":"EAP-AKA challenge sent","imsi":"001010123456789","sessionId":"abc-123"}`.

### crypto/* stdlib

Go's standard library crypto packages (`crypto/aes`, `crypto/cipher`, `crypto/hmac`, `crypto/sha1`, `crypto/sha256`, `crypto/subtle`, `crypto/rand`) cover everything this project needs: AES-ECB for MILENAGE, AES-CBC for EAP attribute encryption, AES-GCM for envelope encryption, HMAC-SHA-1 for AT_MAC, SHA-1 for key derivation, constant-time comparison for security-sensitive comparisons. In TypeScript, we used the `crypto` module too, but Go's versions are more explicitly typed and there's no confusion about Node.js's async vs sync crypto APIs.

### Manual XML building (not encoding/xml)

The XML this server produces is WAP-Provisioning XML -- a rigid, predictable format with `<wap-provisioningdoc>`, `<characteristic>` elements, and `<parm>` elements. Using Go's `encoding/xml` with struct tags would require defining a complex struct hierarchy with nested slices, custom marshaling for optional fields, and careful attribute vs. element control. For a format this rigid, manual string building with proper escaping is actually simpler and more readable:

```go
func parm(name, value string) string {
    return `    <parm name="` + escapeXML(name) + `" value="` + escapeXML(value) + `"/>`
}
```

You can read this and immediately see the output. With `encoding/xml`, you'd be chasing struct tags and wondering why your indentation is wrong.

### testify

`require.Equal(t, expected, actual)` reads better than `if got != want { t.Errorf(...) }`. That's the entire reason. We use testify for readable assertions and nothing else -- no test suites, no mocking framework.

---

## Architecture Deep Dive

### The Request Lifecycle

Let me trace a complete authentication flow, because this is where everything connects.

**Round Trip 1 (RT1): Initial Authentication**

1. A phone sends `POST /entitlement` with `{"imsi": "001010123456789", "app": "ap2004"}`.
2. The chi router dispatches to `EntitlementHandler.HandlePost`.
3. Since there's no `eap_relay` and no `token`, we're on **Path 3: Initial Request**.
4. The `orchestrator.HandleInitialRequest` is called with the IMSI.
5. The orchestrator calls the HSS (via HTTP) to get authentication vectors.
6. The HSS (`cmd/mock-hss/main.go`) looks up the subscriber in Postgres, unwraps the DEK via `crypto.CreateKeyManager`, decrypts Ki and OP, runs the MILENAGE algorithm (`crypto.GenerateVectors`), and returns RAND, AUTN, XRES, CK, IK.
7. Back in the orchestrator: it derives the Master Key (`MK = SHA-1(Identity || IK || CK)`), then runs the FIPS 186-2 PRF to derive session keys `K_encr`, `K_aut`, `MSK`, `EMSK`.
8. It builds an EAP-Request/AKA-Challenge packet with AT_RAND, AT_AUTN, and AT_MAC (HMAC-SHA-1 truncated to 16 bytes).
9. It stores the session state (XRES, keys, etc.) in Redis with a 90-second TTL.
10. Returns 401 with the EAP challenge as base64 in `eap_relay`.

**Round Trip 2 (RT2): Challenge Response**

1. The phone sends `POST /entitlement` with `{"eap_relay": "<base64>"}` and `X-EAP-Session-Id` header.
2. This is **Path 1: EAP Response**. The handler checks the idempotency cache first (replay protection).
3. The `orchestrator.HandleEapResponse` retrieves the session from Redis.
4. It decodes the EAP packet, verifies AT_MAC (timing-safe HMAC comparison), verifies AT_RES == XRES (timing-safe `subtle.ConstantTimeCompare`).
5. On success: looks up the subscriber ID, issues a re-auth identity, stores re-auth state in Redis (48-hour TTL), and returns EAP-Success + entitlement data.

**The 4-Path Routing**

The `HandlePost` function in `routes.go` is the traffic cop. It looks at what fields are present in the request and routes accordingly:

1. `eap_relay` present -> **EAP Response** (RT2 of full auth or re-auth)
2. `token` present -> **Token-based** (re-auth RT1 or ODSA temporary token)
3. Neither -> **Initial Request** (RT1, requires IMSI)

Within Path 1, there's a sub-branch: if a re-auth session exists for the session ID, it's a re-auth RT2; otherwise it's a full-auth RT2. This is determined by checking Redis -- the re-auth session store and the full-auth session store use different key prefixes (`reauth_session:` vs `eap_session:`).

### Dependency Injection: Struct-based, No Framework

Look at the `main()` function in `cmd/ecs/main.go`. Every dependency is created explicitly and wired together by hand:

```go
pool, _ := db.NewPool(ctx, cfg.DatabaseURL, cfg.DBPoolSize)
redisClient, _ := db.NewRedisClient(ctx, cfg.RedisURL)
queries := db.NewQueries(pool)
sessionStore := eapaka.NewSessionStore(redisClient)
reauthStore := eapaka.NewReauthStore(redisClient, config.ReauthStoreTTLSeconds)
orchestrator := eapaka.NewOrchestrator(hssClient, sessionStore, reauthStore, subscriberLookup)
tokenService := token.NewService(queries, redisClient, cfg)
responseBuilder := services.NewResponseBuilder(queries, cfg)
entitlementHandler := server.NewEntitlementHandler(orchestrator, reauthHandler, ...)
srv := server.NewServer(cfg, entitlementHandler)
```

There's no `@Inject` decorator, no `wire.Build`, no dependency injection container. Each struct receives its dependencies through its constructor. This is the Go way: explicit is better than implicit. You can follow the dependency chain from main.go and understand exactly what gets what.

This is a crucial engineering pattern. In TypeScript/Java, DI frameworks scan annotations at runtime to build the dependency graph. That's convenient but opaque -- when something is nil that shouldn't be, you're debugging the framework. In Go, if something is nil, you forgot to pass it in main.go. The bug is in the code you wrote, not in framework magic you didn't write.

### Context Propagation

Every I/O operation takes a `context.Context` as its first argument. This isn't just a convention; it's how Go handles request-scoped cancellation and timeouts. If a client disconnects mid-request, the context gets cancelled, and every downstream operation (Postgres query, Redis lookup, HSS HTTP call) gets notified.

```go
func (h *EntitlementHandler) HandlePost(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()  // The request context
    // ...
    challenge, err := h.orchestrator.HandleInitialRequest(ctx, body.IMSI)
}
```

Inside the orchestrator, that same `ctx` flows into the HSS client's HTTP request:

```go
req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/vectors", ...)
```

If the original request is cancelled, this HTTP call is automatically aborted. In TypeScript, you'd need to manually wire up AbortController. In Go, it's baked into the language's I/O model.

---

## The Crypto Layer: Where It Gets Interesting

### MILENAGE (3GPP TS 35.206)

MILENAGE is the algorithm that SIM cards use to prove their identity. It's a set of functions (f1 through f5, plus f1* and f5*) built on top of AES-128-ECB encryption. The implementation is in `internal/crypto/milenage.go`.

The key insight: a SIM card contains two secrets -- Ki (128-bit subscriber key) and OP (128-bit operator key). Nobody else has these. When the network sends a random number (RAND), the SIM computes a response (RES) using Ki, OP, and RAND. The network does the same computation independently. If the results match, the SIM is authentic.

The Go implementation uses `crypto/aes` for the block cipher and manual `XOR` and `Rotate` operations for the MILENAGE transformations:

```go
// TEMP = AES_K(RAND XOR OPc)
temp := AESEncrypt(ki, XOR(randVal, opc))

// f1: MAC-A for authentication
f1Input := XOR(XOR(Rotate(XOR(in1, opc), r1), temp), c1[:])
out1 := XOR(AESEncrypt(ki, f1Input), opc)
```

This is a faithful translation from the 3GPP spec. Each line corresponds to an equation in TS 35.206 Section 4.1.

### Envelope Encryption

Subscriber secrets (Ki, OP) are never stored in plaintext. The encryption model is:

1. Each subscriber has a unique **DEK** (Data Encryption Key, 32 bytes).
2. Ki and OP are encrypted with the DEK using **AES-256-GCM** (authenticated encryption).
3. The DEK itself is wrapped (encrypted) with a **KEK** (Key Encryption Key).
4. In development: KEK wrapping uses XOR (simple, deterministic, easy to debug).
5. In production: KEK wrapping uses **Google Cloud KMS** (the DEK is encrypted by a key that never leaves Google's HSMs).

The `defer crypto.ZeroSlice(dek)` pattern is worth noting. In TypeScript, you'd write:

```typescript
try {
    const dek = unwrapDek(wrappedDek);
    // use dek...
} finally {
    dek.fill(0);
}
```

Go's `defer` is cleaner and less error-prone -- it's impossible to forget the `finally` block because `defer` is declared right after the value is obtained:

```go
dek, _ := keyManager.UnwrapDEK(ctx, sub.KiDEKWrapped)
defer crypto.ZeroSlice(dek)
// use dek...
```

### EAP Packet Binary Codec

EAP packets are binary. They have a 4-byte header (Code, Identifier, Length), then for EAP-AKA an additional 4-byte sub-header (Type, Subtype, Reserved), followed by TLV attributes that must be 4-byte aligned.

The codec in `internal/eapaka/codec.go` handles the encoding/decoding. The `EncodeAttribute` function is essentially a big switch statement that knows the binary layout of each attribute type:

- **AT_RAND, AT_AUTN, AT_MAC**: 2 reserved bytes + 16 bytes value
- **AT_RES**: 2-byte bit-length prefix + variable value
- **AT_COUNTER**: 2-byte big-endian counter
- **AT_ENCR_DATA**: 2 reserved bytes + variable-length ciphertext

In TypeScript, this was done with `Buffer.alloc()`, `buf.writeUInt16BE()`, `Buffer.from(hex, 'hex')`. In Go, it's `make([]byte, n)`, `binary.BigEndian.PutUint16()`, `hex.DecodeString()`. The operations are equivalent but Go's type system catches more errors at compile time -- you can't accidentally pass a string where bytes are expected.

### Key Derivation

The key derivation follows RFC 4187 Section 7 exactly:

1. **Master Key**: `MK = SHA-1(Identity || IK || CK)` -- simple hash concatenation.
2. **PRF**: FIPS 186-2 SHA-1-based PRF generates 160 bytes of pseudo-random output from the 20-byte MK.
3. **Session Keys**: The first 16 bytes are K_encr (encryption), next 16 are K_aut (authentication), next 64 are MSK, final 64 are EMSK.

The PRF implementation in `keys.go` is interesting because it does 160-bit modular arithmetic (add two 20-byte big-endian numbers with carry):

```go
func add160(a, b []byte) {
    carry := 0
    for i := 19; i >= 0; i-- {
        sum := int(a[i]) + int(b[i]) + carry
        a[i] = byte(sum & 0xff)
        carry = sum >> 8
    }
}
```

This is the kind of code that's easy to get wrong and hard to test without RFC test vectors. The `rfc4187_test.go` file validates this against known-good vectors from the RFC itself.

---

## How Go Improves Over TypeScript

### Goroutines vs. Event Loop

In Node.js, there's one thread running your JavaScript. When 80 requests arrive simultaneously, they all share that thread via the event loop. If any handler does CPU-intensive work (like MILENAGE's AES operations), it blocks everyone else.

In Go, every HTTP request automatically gets its own goroutine. Goroutines are lightweight -- a few KB of stack space, managed by Go's runtime scheduler across OS threads. 500 concurrent requests means 500 goroutines, each independently running their crypto computations on separate OS threads. No event loop. No `async/await`. No callback hell.

When you write:

```go
func (h *EntitlementHandler) HandlePost(w http.ResponseWriter, r *http.Request) {
    result, err := h.orchestrator.HandleEapResponse(ctx, body.EapRelay, sessionID)
}
```

That `HandleEapResponse` call does Redis lookups, HMAC verification, and subscriber lookup -- all of which involve network I/O. In Go, the goroutine simply blocks on each I/O call and the runtime schedules another goroutine to run. In TypeScript, each of those would need `await` and the function would need to be `async`. The mental model is simpler in Go.

### Memory and Cold Starts

The TypeScript version needs the V8 engine (~100MB baseline), plus node_modules (~40MB), plus TypeBox schema compilation at startup, plus Drizzle ORM initialization. Total memory: ~1GB. Cold start time: 2-3 seconds.

The Go version is a single statically-compiled binary. The distroless Docker image is essentially just this binary. Memory usage: ~256MB under load. Cold start: ~100ms. This matters enormously on Cloud Run, where you pay for cold starts and memory.

### Binary Deployment

The Dockerfile tells the whole story:

```dockerfile
FROM golang:1.24-bookworm AS builder
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /ecs ./cmd/ecs

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /ecs /ecs
ENTRYPOINT ["/ecs"]
```

Multi-stage build: compile in a full Go image, copy the single binary to a minimal distroless image. No node_modules. No runtime. No package manager. No shell. The container has almost no attack surface.

### Type Safety at Compile Time

In TypeScript, TypeBox validates request schemas at runtime. If you typo a field name, you find out when a request arrives. In Go, the `entitlementPostRequest` struct has compile-time field types:

```go
type entitlementPostRequest struct {
    App       string `json:"app"`
    IMSI      string `json:"imsi,omitempty"`
    Token     string `json:"token,omitempty"`
    EapRelay  string `json:"eap_relay,omitempty"`
}
```

If you try to do `body.app` (lowercase) instead of `body.App`, the compiler catches it.

---

## Key Lessons and Bugs We Encountered

### 1. Go's Multiple Return Values Bite You Quietly

`crypto.CreateKeyManager()` returns `(KeyManager, error)`. In TypeScript, you'd get a rejected promise if something went wrong, and your try/catch would handle it. In Go, it's easy to write:

```go
keyManager, _ := crypto.CreateKeyManager(...)
// If there was an error, keyManager is nil, and you'll get a nil pointer panic later
```

The lesson: *never* discard errors from functions that create important resources. The `_` discard should only be used for errors you've genuinely thought about (like a Redis cache write failure that you can survive without).

### 2. Struct Field Naming Conventions

TypeScript: `sqnMs`, `sqnMS`, `SQNMS` -- all valid, style is a choice.

Go: exported fields *must* start with uppercase, and the convention for acronyms is to capitalize only the first letter when the acronym isn't the entire name. So it's `SqnMs`, not `SQNMS`. Get this wrong and `json.Unmarshal` silently doesn't populate the field because Go considers it unexported. This is a subtle bug that produces no error -- just zero values.

### 3. Pointer Types for Optional Database Fields

In TypeScript, optional columns are handled with `string | null` or optional chaining (`subscriber?.msisdn`). In Go, a database column that can be NULL needs a pointer type:

```go
type Subscriber struct {
    MSISDN  *string   `json:"msisdn"`    // Can be NULL
    IMSI     string   `json:"imsi"`      // NOT NULL
}
```

If you use `string` for a nullable column, `pgx.Scan` will error when it encounters NULL. This tripped us up early -- the TypeScript version hid this behind Drizzle's ORM layer.

### 4. gofmt Formatting Drift

7 files needed formatting fixes, caught by the CI `fmt-check` gate. In TypeScript, Prettier reformats on save. In Go, `gofmt` is the canonical formatter, but if your editor isn't configured to run it on save, formatting drifts. The Makefile has both `fmt` (fix) and `fmt-check` (CI gate):

```makefile
fmt-check:
    @test -z "$$(gofmt -l .)" || (echo "Files not formatted:" && gofmt -l . && exit 1)
```

Run `make fmt` before committing. Always.

### 5. Buffer Handling Translation

TypeScript's Buffer API vs Go's binary package:

| TypeScript | Go |
|---|---|
| `buf.readUInt16BE(offset)` | `binary.BigEndian.Uint16(buf[offset:])` |
| `buf.writeUInt16BE(value, offset)` | `binary.BigEndian.PutUint16(buf[offset:], value)` |
| `Buffer.from(hexStr, 'hex')` | `hex.DecodeString(hexStr)` |
| `buf.toString('base64')` | `base64.StdEncoding.EncodeToString(buf)` |
| `Buffer.alloc(n)` | `make([]byte, n)` |

The Go versions are more verbose but less ambiguous. `buf.readUInt16BE(offset)` doesn't tell you the slice bounds; `binary.BigEndian.Uint16(buf[offset:offset+2])` does.

### 6. defer for Secret Zeroing

Go's `defer` is perfect for memory hygiene patterns:

```go
dek, err := keyManager.UnwrapDEK(ctx, sub.KiDEKWrapped)
if err != nil { return err }
defer crypto.ZeroSlice(dek)  // Guaranteed to run when function exits
```

The TypeScript equivalent with `try/finally` is more error-prone because the `finally` block is physically distant from the variable declaration. Engineers forget to add it, or they add an early return that bypasses it. `defer` is declared right next to the allocation, and it works regardless of how the function exits (return, panic, etc.).

### 7. Goroutines Are Free Concurrency

In TypeScript, every handler is on the event loop. If two requests arrive, they interleave their async operations. In Go, each request is automatically a goroutine:

```go
r.Post("/entitlement", entitlementHandler.HandlePost)
// Every request to this route runs HandlePost in its own goroutine
```

No `async` keyword. No `.then()`. No `Promise.all()` for parallel operations. The function just runs, blocks when it needs to wait for I/O, and the runtime handles the multiplexing. This is a paradigm shift from Node.js -- you write synchronous-looking code that's actually highly concurrent.

### 8. chi Middleware Chaining

Express middleware: `app.use(middleware)`. chi middleware: `r.Use(middleware)`. Same concept, same ordering (first registered = outermost wrapper). The chi middleware signature is `func(http.Handler) http.Handler` -- a function that takes a handler and returns a wrapped handler. It's closures all the way down.

---

## How Good Engineers Think About This Code

### Protocol Fidelity Over Cleverness

Notice how the EAP codec doesn't try to be clever. Each attribute type has explicit encode/decode logic:

```go
case config.ATRand, config.ATAutn, config.ATMac:
    valuePayload = make([]byte, 18)
    copy(valuePayload[2:], attr.Value[:16])
```

A "clever" engineer might try to generalize this into a table-driven system. But these attribute formats are defined by RFC 4187, and each one has unique quirks (AT_RES has a bit-length prefix, AT_COUNTER is just 2 bytes, AT_ENCR_DATA has variable length). Encoding the specific behavior of each attribute type is more maintainable than a generic system that would need edge cases for every type anyway.

The lesson: when implementing a spec, match the spec's structure. The code should read like the RFC.

### Timing-Safe Comparisons Everywhere

Every cryptographic comparison uses `subtle.ConstantTimeCompare`:

```go
// AT_RES verification
subtle.ConstantTimeCompare(atRes.Value, session.XRES) != 1

// AT_MAC verification
subtle.ConstantTimeCompare(computed, receivedMAC) == 1
```

An ordinary `==` comparison short-circuits: it returns false as soon as it finds the first differing byte. An attacker measuring response times can deduce how many leading bytes are correct and progressively discover the secret. `ConstantTimeCompare` always takes the same amount of time regardless of which bytes differ. This is a security best practice that's easy to forget and impossible to catch in testing (timing differences are nanoseconds).

### Fail Fast, Clean Up Always

Look at the orchestrator's `HandleEapResponse`:

```go
if session.State != config.EAPStateChallengeSent {
    _ = o.sessionStore.Delete(ctx, sessionID)
    return eapFailureResult(session.Identifier), nil
}
```

Every failure path deletes the session. There's no "dangling session" that could be exploited for replay attacks. The pattern is consistent: validate, fail with cleanup, or succeed. Every validation step either returns an error (with cleanup) or falls through to the next check.

### Idempotency for Free

The idempotency cache is a subtle but important detail:

```go
cached, _ := h.idempotencyCache.GetCachedResponse(ctx, sessionID, body.EapRelay)
if cached != nil {
    // Return the cached response -- no reprocessing
}
```

The cache key is `SHA-256(sessionID + eapRelay)`. If the client retransmits the exact same EAP response (network glitch, timeout retry), we return the same result without re-running the authentication logic. This prevents double-issuance of re-auth identities and avoids confusing state transitions.

### Redis Key Design

The Redis key patterns are deliberate:

- `eap_session:{uuid}` -- full auth session, 90s TTL
- `reauth_session:{uuid}` -- re-auth session, 90s TTL
- `reauth:{id}` -- long-lived re-auth state, 48hr TTL
- `token:{value}` -- token cache, matches token TTL
- `eap_idempotency:{sha256}` -- idempotency cache, 90s TTL

The prefixes prevent key collisions. The TTLs ensure automatic cleanup. No background job needed to expire old sessions -- Redis does it for free.

---

## The Service Handler Pattern

Each of the 12 telecom services follows the same pattern. They're pure functions that take status/config data in and return an `ApplicationConfig` out:

```go
func BuildVoWiFiConfig(status, provStatus, tcStatus int, configData *VoWiFiConfigData) *protocol.ApplicationConfig {
    result := &protocol.ApplicationConfig{
        AppID:             "ap2004",
        EntitlementStatus: status,
        TcStatus:          protocol.IntPtr(tcStatus),
        ProvStatus:        protocol.IntPtr(provStatus),
    }
    if status == protocol.EntitlementStatusEnabled {
        result.AddrStatus = protocol.IntPtr(1)
        result.Addresses = configData.Addresses  // ePDG, P-CSCF addresses
    }
    return result
}
```

The central switch in `builder.go` routes each `appID` to its handler:

```go
case config.AppIDVoWiFi:     return BuildVoWiFiConfig(...)
case config.AppIDVoLTE:      return BuildVoLTEConfig(...)
case config.AppIDSMSoIP:     return BuildSmsOipConfig(...)
// ... 9 more handlers
```

Adding a new service means: (1) define the config data struct, (2) write the builder function, (3) add the case to the switch. Three files touched, no framework to learn.

---

## What Changed Between TypeScript and Go (And What Didn't)

**Stayed the same:**
- Wire format: identical EAP packets, identical JSON/XML responses
- Redis key patterns: same prefixes, same TTLs
- Database schema: identical 5-table structure
- API contract: same `POST /entitlement` and `GET /entitlement` endpoints
- Authentication flow: same 4-path routing, same re-auth logic, same SQN resync

**Changed:**
- Async/await replaced by goroutines (synchronous-looking concurrent code)
- TypeBox runtime validation replaced by compile-time type checking
- Drizzle ORM replaced by 10 raw SQL queries
- Pino replaced by stdlib `log/slog`
- Express/Fastify middleware replaced by chi middleware (same semantics, different syntax)
- node_modules + V8 replaced by a single static binary in a distroless container
- Manual try/finally for secret zeroing replaced by `defer`
- `Buffer` API replaced by `encoding/binary` + `[]byte` slices

The protocol is the same. The infrastructure is different. That's the whole point: when you re-implement in a new language, you want to change the *engine* without changing the *behavior*.

---

## Running It

```bash
# Build both binaries
make build

# Run tests
make test

# Run with race detector (catch concurrency bugs)
make test-race

# Format check (CI gate)
make fmt-check

# Docker
make docker-up    # starts ECS, mock-hss, Postgres, Redis
make docker-down  # tears it down
```

The Makefile is deliberately simple. Each target is one command. No conditional logic, no platform detection. If you can read `go build -ldflags="-s -w" -o bin/ecs ./cmd/ecs`, you know what `make build-ecs` does.

---

## The Style Guide Audit: Finding Hidden Bugs in "Working" Code

After the entire codebase was functional and all tests passing, we ran a comprehensive audit against the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md) and Google's Go best practices. This is the engineering equivalent of a home inspection after construction — the house looks fine from the outside, but the inspector finds wiring that could start a fire.

The audit found issues at three priority levels, and the fixes touched 27 files. Here's what we found and why it matters.

### HIGH Priority: The Bugs That Could Bite You in Production

**1. A `panic` hiding in the crypto layer**

The `AESEncrypt` function in `milenage.go` called `aes.NewCipher(key)` and panicked if it failed:

```go
// BEFORE: a panic bomb waiting to go off
func AESEncrypt(key, input []byte) []byte {
    block, err := aes.NewCipher(key)
    if err != nil {
        panic(err) // This kills the entire server process
    }
    out := make([]byte, 16)
    block.Encrypt(out, input)
    return out
}
```

In production, if a corrupted key somehow made it through (database corruption, memory error, bad migration), this would crash the entire ECS process — not just the one request, but every concurrent request being served. In Go, `panic` unwinds the entire goroutine stack, and if not recovered, takes down the process.

The fix cascaded through 7 functions. `AESEncrypt` now returns `([]byte, error)`, which meant `ComputeOPc`, `GenerateVectorsWithRAND`, `F5Star`, `F1Star`, `ValidateAUTS`, and `GenerateAUTS` all needed to propagate the error. This is a textbook example of why the Uber guide says "Don't panic" — a single panic in a leaf function forces error handling redesign across the entire call chain when you eventually fix it.

```go
// AFTER: errors propagate, callers decide what to do
func AESEncrypt(key, input []byte) ([]byte, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, fmt.Errorf("aes.NewCipher: %w", err)
    }
    out := make([]byte, 16)
    block.Encrypt(out, input)
    return out, nil
}
```

The lesson: `panic` is for truly unrecoverable situations (programmer bugs, not runtime conditions). If there's *any* path where the input could be invalid — including paths you haven't thought of yet — return an error.

**2. The audit goroutine that silently dropped every log entry**

The audit logging middleware launched a goroutine to write audit logs to Postgres — a sensible fire-and-forget pattern. But it used the *request context*:

```go
// BEFORE: uses r.Context(), which is canceled when the handler returns
go func() {
    err := queries.InsertAuditLog(r.Context(), &db.AuditLog{...})
    // This error was ALWAYS "context canceled" because the HTTP response
    // was already sent, which cancels r.Context()
}()
```

The HTTP handler returns the response, Go cancels the request context, and the goroutine's database write fails silently. Every single audit log was being dropped. The code looked correct, the server ran fine, but the audit table was empty. This is the kind of bug that passes every test (because tests don't check for audit logs) and only gets discovered during a compliance review months later.

```go
// AFTER: background context with its own timeout
go func() {
    bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := queries.InsertAuditLog(bgCtx, &db.AuditLog{...}); err != nil {
        slog.Error("Failed to write audit log", "err", err)
    }
}()
```

The lesson: goroutines that outlive the request must use `context.Background()`, not the request context. This is a common Go mistake — the request context is designed to cancel when the response is sent, which is exactly what you *don't* want for background work.

**3. Error masking in the token service**

The token validation function treated every error as "token not found":

```go
// BEFORE: database failures look like invalid tokens
tokenRow, err := s.queries.FindTokenByValue(ctx, tokenValue)
if err != nil {
    return nil, ErrTokenNotFound  // Redis down? "Token not found." Postgres timeout? "Token not found."
}
```

If Redis or Postgres was having issues, every authenticated user would get "invalid token" errors instead of 500 Internal Server Error. Users would think their tokens expired when really the database was down. Debugging would be a nightmare because the error message points in the completely wrong direction.

```go
// AFTER: distinguish "not found" from "infrastructure failure"
tokenRow, err := s.queries.FindTokenByValue(ctx, tokenValue)
if errors.Is(err, db.ErrTokenNotFound) {
    return nil, ErrTokenNotFound
}
if err != nil {
    return nil, fmt.Errorf("query token: %w", err)
}
```

The lesson: sentinel errors (`ErrNotFound`) exist for a reason. Always check for them specifically before falling through to a generic error handler. The Uber guide calls this the "handle errors once" rule — each error should be handled exactly once, at the right level.

### MEDIUM Priority: Code Smells That Compound Over Time

**18 bare `return err` statements** were wrapped with context. Before:

```go
func (s *ReauthStore) Delete(ctx context.Context, reauthID string) error {
    return s.client.Del(ctx, reauthStoreKey(reauthID)).Err()
}
```

After:

```go
func (s *ReauthStore) Delete(ctx context.Context, reauthID string) error {
    if err := s.client.Del(ctx, reauthStoreKey(reauthID)).Err(); err != nil {
        return fmt.Errorf("delete reauth state %s: %w", reauthID, err)
    }
    return nil
}
```

Why does this matter? When you see `delete reauth state abc-123: connection refused` in a log, you know exactly which operation failed and which key was involved. When you see `connection refused`, you're grepping the entire codebase for every Redis call. At 3am during an outage, the difference is minutes vs. hours.

**Line-of-sight refactoring** flattened deeply nested code. The principle: the happy path should be at the left margin. Error handling should be indented. This makes code scannable — you can read down the left edge and understand the success flow without mentally tracking indentation levels.

```go
// BEFORE: happy path buried inside else branches
if status == protocol.EntitlementStatusEnabled {
    result.AddrStatus = protocol.IntPtr(1)
    result.Addresses = configData.Addresses
} else {
    result.AddrStatus = protocol.IntPtr(0)
}

// AFTER: guard clause, happy path at top level
if status != protocol.EntitlementStatusEnabled {
    result.AddrStatus = protocol.IntPtr(0)
    return result
}
result.AddrStatus = protocol.IntPtr(1)
result.Addresses = configData.Addresses
```

But here's a gotcha we hit: **guard clauses can break logic when there's post-branch code.** The VoWiFi handler had a T&C status check that ran *after* both branches of the if/else. Converting to a guard clause with early return skipped that check, and a test caught it. The lesson: refactor mechanically, but always verify that post-branch logic doesn't depend on both paths completing.

**Signal handler lifecycle management** was tightened. The original pattern started a goroutine to listen for SIGTERM and shut down the server, but there was no way to know when shutdown completed:

```go
// BEFORE: fire-and-forget — main() might exit before shutdown finishes
go func() {
    <-sigCh
    srv.Shutdown(ctx)  // Error silently discarded
}()
```

```go
// AFTER: channel-based lifecycle — main() waits for clean shutdown
shutdownErr := make(chan error, 1)
go func() {
    <-sigCh
    shutdownErr <- srv.Shutdown(ctx)
}()

if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) { ... }
if err := <-shutdownErr; err != nil {
    slog.Error("Server shutdown error", "err", err)
}
```

The Uber guide rule: "No goroutine without an exit strategy." The channel gives the main goroutine a way to wait for shutdown and capture any error. In the old version, a shutdown error (like an in-flight request timeout) would be silently lost.

**A 101-line function was split into three.** `handleEapRelayPath` was doing three different things depending on the request type. It was split into `handleEapRelayPath` (dispatcher, 30 lines), `handleReauthEapRelay` (re-auth RT2, 38 lines), and `handleFullAuthEapRelay` (full auth RT2, 36 lines). Each function now has a single responsibility and fits on one screen.

**Generics eliminated copy-paste in the response builder.** The `buildAppConfig` function had 8 identical blocks:

```go
// BEFORE: same 4 lines repeated 8 times with different types
case config.AppIDVoWiFi:
    var cd VoWiFiConfigData
    if err := json.Unmarshal(configData, &cd); err != nil {
        slog.Warn("failed to unmarshal", "appId", appID, "err", err)
    }
    return BuildVoWiFiConfig(status, provStatus, tcStatus, &cd)
```

```go
// AFTER: one generic helper, each case is one line
func unmarshalConfig[T any](configData json.RawMessage, appID string) *T {
    var cd T
    if err := json.Unmarshal(configData, &cd); err != nil {
        slog.Warn("failed to unmarshal config data", "appId", appID, "err", err)
    }
    return &cd
}

case config.AppIDVoWiFi:
    return BuildVoWiFiConfig(status, provStatus, tcStatus, unmarshalConfig[VoWiFiConfigData](configData, appID))
```

Go 1.18 generics used surgically: one 7-line helper eliminated 32 lines of repetition without adding abstraction complexity.

### LOW Priority: Polish That Separates Professional Code from "It Works" Code

**Function ordering**: Go convention is exported functions first, unexported helpers after. Eight files had unexported helpers (`sessionKey`, `reauthStoreKey`, `idempotencyKey`, etc.) interleaved with exported methods. Reordering them makes the API surface immediately visible when you open a file — you see the public contract first, implementation details second.

**Variable naming**: Removed redundant `*Bytes` suffixes from tightly-scoped variables. In a 20-line function where `rand` is clearly a `[]byte` from `base64.DecodeString`, calling it `randBytes` adds noise without information. Also shortened `operationTypeStr` to `opTypeRaw` and `acceptContentType` to `ct` where the scope was tight enough.

**Doc comments on ~70 exported constants**: Every exported constant now has a `// SymbolName is...` comment referencing the relevant spec (RFC 4187, RFC 3748, GSMA TS.43). This isn't just style — `go doc` and IDE hover tooltips show these comments, so anyone using these constants sees the RFC section number without opening the spec.

### The Meta-Lesson: Style Guides Find Real Bugs

The audit started as a code style exercise and found three production-grade bugs (panic, canceled context, error masking). This isn't coincidence. Style guide rules like "don't panic," "handle errors once," and "goroutines need exit strategies" exist *because* violating them causes these exact bugs.

When someone says "style is just cosmetics," remind them: the audit goroutine was dropping every log entry, and it looked perfectly fine at a glance. The style guide rule about context management would have caught it at review time. Style guides are encoded experience from engineers who already got burned.

---

## Final Thoughts

Re-implementing a working system in a new language is a unique engineering exercise. You already know what the system does, so you can focus entirely on *how* to express it idiomatically in the new language. The TypeScript version taught us the domain (telecom entitlements, EAP-AKA, MILENAGE, TS.43). The Go version taught us how to build the same thing with better performance characteristics and more explicit control over concurrency, memory, and deployment.

The biggest lesson? **Explicitness is a feature, not a tax.** Go makes you write more code, but that code is easier to debug, easier to trace, and easier to understand at 3am when production is down. The TypeScript version is more concise. The Go version is more transparent. Both are valid trade-offs; the right choice depends on your team and your constraints.

For a server that handles SIM card authentication at scale, where every function has cryptographic security implications and every request is a goroutine, Go's explicitness is the right trade-off.
