 Here is Claude's plan:
╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌
 Go Re-implementation Plan: Entitlements-as-a-Service

 Context

 The Entitlements-as-a-Service project is a GSMA TS.43 compliant Entitlement Configuration Server (ECS) that authenticates mobile devices using SIM card hardware tokens via EAP-AKA (RFC 4187). The current
 TypeScript implementation (~3300 LOC across ECS + Mock HSS) is fully functional and deployed on GCP Cloud Run.

 We're re-implementing in Go for: better concurrency (goroutines vs single-threaded Node.js), lower memory footprint (~256MB vs ~1GB), faster cold starts, and higher per-instance throughput (~500 concurrent
 requests vs ~80).

 The Go version must be protocol-identical — same wire-format EAP packets, same Redis key patterns, same API contract, same database schema.

 ---
 Technology Stack
 ┌─────────────┬─────────────────────────────┬───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
 │   Concern   │           Choice            │                                                               Rationale                                                               │
 ├─────────────┼─────────────────────────────┼───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
 │ HTTP Router │ chi                         │ Lightweight, idiomatic middleware chaining, close to stdlib. No framework lock-in.                                                    │
 ├─────────────┼─────────────────────────────┼───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
 │ Database    │ pgx/v5 + raw SQL            │ Native PostgreSQL driver with connection pooling. Direct []byte for bytea, json.RawMessage for jsonb. See note below on why not GORM. │
 ├─────────────┼─────────────────────────────┼───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
 │ Redis       │ go-redis/redis/v9           │ Mature client, supports hashes/expiry/pipelines. Direct replacement for ioredis.                                                      │
 ├─────────────┼─────────────────────────────┼───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
 │ Validation  │ go-playground/validator/v10 │ Struct tag validation, idiomatic Go. Replaces TypeBox/Ajv.                                                                            │
 ├─────────────┼─────────────────────────────┼───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
 │ Logging     │ log/slog (stdlib)           │ Structured JSON logging, maps to Cloud Logging severity. Zero dependencies.                                                           │
 ├─────────────┼─────────────────────────────┼───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
 │ Tracing     │ go.opentelemetry.io/otel    │ Same OTel ecosystem as TypeScript version. First-class Go support.                                                                    │
 ├─────────────┼─────────────────────────────┼───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
 │ Crypto      │ crypto/* (stdlib)           │ sha1, hmac, aes, cipher, subtle — everything needed. No external crypto libs.                                                         │
 ├─────────────┼─────────────────────────────┼───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
 │ Testing     │ stdlib testing + testify    │ testify/assert for readable assertions. No test framework beyond stdlib.                                                              │
 ├─────────────┼─────────────────────────────┼───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
 │ XML         │ Manual string building      │ WAP-Provisioning format is rigid/predictable. encoding/xml struct tags would add complexity for no benefit.                           │
 ├─────────────┼─────────────────────────────┼───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
 │ JSON        │ encoding/json (stdlib)      │ Response shapes are simple. Drop-in replaceable with go-json if needed later.                                                         │
 ├─────────────┼─────────────────────────────┼───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
 │ Cloud KMS   │ cloud.google.com/go/kms     │ Production envelope encryption. Lazy-loaded.                                                                                          │
 ├─────────────┼─────────────────────────────┼───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
 │ Linting     │ golangci-lint               │ Standard Go linting aggregator. Replaces ESLint.                                                                                      │
 └─────────────┴─────────────────────────────┴───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
 Why not GORM?

 GORM was considered but rejected for this project because:
 1. Simple, fixed queries — The ECS has ~10 distinct queries with no dynamic building or complex joins. An ORM's query builder adds no value.
 2. Binary data — Heavy use of bytea (ki_encrypted, op_encrypted, ki_dek_wrapped) and jsonb (config_data). pgx handles these natively as []byte and json.RawMessage without an abstraction layer that can make
 binary handling awkward.
 3. Performance — Every EAP-AKA handshake hits DB multiple times. GORM's reflection-based scanning adds per-query overhead that compounds at 2000 RPS target.
 4. Security transparency — In a system handling SIM card secrets (Ki/OP), you want to see exactly what SQL runs. GORM's auto-migrations, hooks, and soft deletes are footguns in this context.
 5. Precedent — The TypeScript version uses Drizzle (query builder, not full ORM) deliberately.

 ---
 Project Structure

 entitlements-as-a-service-poc-go/
 ├── go.mod
 ├── go.sum
 ├── cmd/
 │   ├── ecs/main.go                    # ECS server entry point
 │   └── mock-hss/main.go              # Mock HSS entry point
 ├── internal/
 │   ├── config/
 │   │   ├── config.go                  # Config struct, env var loading
 │   │   └── constants.go              # EAP-AKA, AppIDs, states, TTLs
 │   ├── crypto/
 │   │   ├── milenage.go               # MILENAGE f1-f5, f1*, f5*
 │   │   ├── milenage_test.go          # 3GPP TS 35.207 test vectors
 │   │   ├── envelope.go               # AES-256-GCM encrypt/decrypt, DEK wrap/unwrap, zeroSlice
 │   │   ├── envelope_test.go
 │   │   ├── kms.go                    # KeyManager interface + LocalKeyManager + CloudKmsKeyManager
 │   │   └── kms_test.go
 │   ├── eapaka/
 │   │   ├── codec.go                  # EAP packet encode/decode, attribute TLV (4-byte aligned)
 │   │   ├── codec_test.go
 │   │   ├── keys.go                   # MK derivation, FIPS 186-2 PRF, MAC compute/verify
 │   │   ├── keys_test.go
 │   │   ├── encryption.go            # AES-128-CBC for AT_ENCR_DATA (manual AT_PADDING)
 │   │   ├── encryption_test.go
 │   │   ├── orchestrator.go          # Full auth state machine (RT1 challenge, RT2 response, SQN resync)
 │   │   ├── reauth.go                # Fast re-auth (RFC 4187 §5.1)
 │   │   ├── session.go               # Redis EAP session CRUD (90s TTL)
 │   │   ├── reauth_session.go        # Redis re-auth session CRUD (90s TTL)
 │   │   ├── reauth_store.go          # Redis re-auth state (48h TTL)
 │   │   ├── idempotency.go           # Redis response replay cache (90s TTL)
 │   │   └── vectors.go               # HTTP client to mock-hss (/vectors, /resync)
 │   ├── db/
 │   │   ├── postgres.go              # pgxpool setup
 │   │   ├── redis.go                 # go-redis client setup
 │   │   ├── models.go                # Row structs (Subscriber, Device, Entitlement, Token, AuditLog)
 │   │   ├── queries.go               # All SQL queries
 │   │   ├── migrate.go               # Schema bootstrap from sql/schema.sql
 │   │   └── seed.go                  # Test data seeder (subscribers + entitlements)
 │   ├── token/
 │   │   ├── service.go               # Generate/validate tokens, write-through cache (Postgres + Redis)
 │   │   └── service_test.go
 │   ├── protocol/
 │   │   ├── apptypes.go              # AppID constants (ap2003-ap2016), ODSA operations
 │   │   ├── statuscodes.go           # EntitlementStatus, ProvStatus, TcStatus, SubscriptionResult
 │   │   ├── request.go               # EntitlementRequest struct + validator tags
 │   │   ├── response.go              # ServiceEntitlementResponse, ApplicationConfig, AddressConfig
 │   │   ├── json_builder.go          # TS.43 JSON response formatter
 │   │   ├── json_builder_test.go
 │   │   ├── xml_builder.go           # WAP-Provisioning XML formatter
 │   │   └── xml_builder_test.go
 │   ├── services/
 │   │   ├── builder.go               # Route appId → handler, query DB, format response
 │   │   ├── builder_test.go
 │   │   ├── vowifi.go                # ap2004
 │   │   ├── volte.go                 # ap2003
 │   │   ├── smsoip.go                # ap2005
 │   │   ├── odsa_common.go           # Shared ODSA helpers + buildOdsaBaseConfig
 │   │   ├── odsa_companion.go        # ap2006 (6 operations)
 │   │   ├── odsa_primary.go          # ap2009 (7 operations)
 │   │   ├── data_plan.go             # ap2010 (3 operations)
 │   │   ├── server_odsa.go           # ap2011 (3 operations)
 │   │   ├── dcb.go                   # ap2012
 │   │   ├── private_identity.go      # ap2013
 │   │   ├── device_user_info.go      # ap2014 (2 operations)
 │   │   ├── app_auth.go              # ap2015
 │   │   ├── sat_mode.go              # ap2016
 │   │   └── mock_smdp.go            # Canned eSIM activation codes
 │   ├── server/
 │   │   ├── server.go                # chi router, middleware registration, graceful shutdown
 │   │   ├── routes.go                # POST/GET /entitlement, GET /health
 │   │   ├── middleware.go            # requestParser, userAgent, versionCheck, errorHandler
 │   │   └── audit.go                 # Audit logging middleware
 │   └── observability/
 │       ├── logger.go                # slog with Cloud Logging severity
 │       ├── tracing.go               # OTel trace provider
 │       └── metrics.go               # OTel meter provider
 ├── sql/
 │   └── schema.sql                   # DDL for all 5 tables
 ├── Dockerfile.ecs
 ├── Dockerfile.mock-hss
 ├── docker-compose.yml               # Postgres + Redis + mock-hss + ecs
 ├── Makefile                          # build, test, lint, seed, docker-up
 └── .github/workflows/ci.yml

 ---
 Implementation Phases

 Phase 1: Project Scaffold + Crypto Primitives

 Goal: Go module init, all pure crypto functions, validated against known test vectors.

 Files:
 - go.mod, go.sum
 - internal/config/constants.go — all EAP-AKA constants, AppIDs, states, attribute types
 - internal/crypto/milenage.go — port from mock-hss/src/milenage.ts: computeOPc, generateVectors, generateVectorsWithRand, f1Star, f5Star, validateAuts
 - internal/crypto/milenage_test.go — 3GPP TS 35.207 official test vectors (from mock-hss/tests/milenage.test.ts)
 - internal/crypto/envelope.go — AES-256-GCM encrypt/decrypt, DEK wrap/unwrap, zeroSlice
 - internal/crypto/envelope_test.go
 - internal/crypto/kms.go — KeyManager interface, LocalKeyManager (XOR for dev), CloudKmsKeyManager
 - internal/eapaka/codec.go — port from src/auth/eapCodec.ts: EncodeEapPacket, DecodeEapPacket, all attribute encode/decode (AT_RAND, AT_AUTN, AT_RES, AT_AUTS, AT_MAC, AT_IV, AT_ENCR_DATA, AT_COUNTER,
 AT_NONCE_S, AT_NEXT_REAUTH_ID, AT_PADDING, AT_COUNTER_TOO_SMALL)
 - internal/eapaka/codec_test.go — port from tests/unit/eapCodec.test.ts + tests/unit/rfc4187Compliance.test.ts
 - internal/eapaka/keys.go — port from src/auth/keyDerivation.ts: BuildIdentity, DeriveMasterKey, PrfSHA1 (FIPS 186-2), DeriveKeys, ComputeMAC, VerifyMAC, DeriveReauthKeys
 - internal/eapaka/keys_test.go — port from tests/unit/keyDerivation.test.ts
 - internal/eapaka/encryption.go — port from src/auth/eapEncryption.ts: EncryptAttributes, DecryptAttributes (AES-128-CBC, manual AT_PADDING)
 - internal/eapaka/encryption_test.go — port from tests/unit/eapEncryption.test.ts

 Validation: All unit tests pass. MILENAGE produces identical outputs to TypeScript for 3GPP test vectors.

 ---
 Phase 2: Config + Database + Redis

 Goal: Configuration loading, DB/Redis connectivity, schema creation, seed data.

 Files:
 - internal/config/config.go — Config struct matching all env vars from src/config/index.ts (same names, same defaults)
 - internal/db/postgres.go — NewPool(cfg) → *pgxpool.Pool
 - internal/db/redis.go — NewRedisClient(cfg) → *redis.Client
 - internal/db/models.go — Go structs for all 5 tables
 - internal/db/queries.go — FindSubscriberByIMSI, FindEntitlementsBySubscriber, InsertToken, FindTokenByValue, InsertAuditLog, etc.
 - sql/schema.sql — DDL extracted from Drizzle schema (src/db/schema.ts)
 - internal/db/migrate.go — reads and executes schema.sql
 - internal/db/seed.go — port from mock-hss/src/seed.ts + src/db/seed-entitlements.ts (idempotent upserts)

 Validation: Connect to Postgres/Redis, create tables, seed data, query it back.

 ---
 Phase 3: Session Management + HSS Client

 Goal: Redis session stores with identical key patterns, HTTP client to mock-hss.

 Files:
 - internal/eapaka/session.go — Redis hash eap_session:{id} (90s TTL). Port from src/auth/eapSession.ts
 - internal/eapaka/reauth_session.go — Redis hash reauth_session:{id} (90s TTL). Port from src/auth/reauthSession.ts
 - internal/eapaka/reauth_store.go — Redis hash reauth:{id} (48h TTL). Port from src/auth/reauthStore.ts
 - internal/eapaka/idempotency.go — Redis string eap_idempotency:{sha256} (90s TTL). Port from src/auth/eapIdempotency.ts
 - internal/eapaka/vectors.go — HSSClient with FetchVectors and ResyncVectors. Port from src/auth/eapAkaVectors.ts

 Key detail: All Redis key prefixes and TTLs must be identical to TypeScript. Binary fields stored as base64 strings in Redis hashes.

 Validation: Session CRUD produces identical Redis keys/values.

 ---
 Phase 4: EAP-AKA Orchestrator + Fast Re-auth

 Goal: Complete EAP-AKA state machine — the core of the project.

 Files:
 - internal/eapaka/orchestrator.go — port from src/auth/eapAka.ts:
   - HandleInitialRequest: fetch vectors → derive keys → build EAP-Request/AKA-Challenge → store session → return 401
   - HandleEapResponse: get session → decode packet → handle AUTH_REJECT/SYNC_FAILURE/CHALLENGE → verify AT_RES (timing-safe) + AT_MAC → issue re-auth identity → return 200
 - internal/eapaka/reauth.go — port from src/auth/eapReauth.ts:
   - HandleReauthRequest: get re-auth state → build encrypted inner attrs → EAP-Request/AKA-Reauthentication → store session → return 401
   - HandleReauthResponse: verify MAC → decrypt inner attrs → check counter → derive new keys → rotate state → return 200

 Critical details:
 - EAP identifier counter: atomic.AddUint32(&counter, 1) % 256
 - MAC patching: compute MAC over packet with zeroed MAC field, then copy() real MAC into packet bytes
 - subtle.ConstantTimeCompare(atRes, xres) for timing-safe verification
 - SQN resync: on SYNC_FAILURE, extract AT_AUTS, call ResyncVectors, build new challenge

 Validation: Full EAP-AKA handshake (RT1+RT2) and fast re-auth (RT1+RT2) against mock-hss.

 ---
 Phase 5: Token Service + Protocol Layer

 Goal: Token management and TS.43 response formatting.

 Files:
 - internal/token/service.go — port from src/auth/tokenService.ts: GenerateToken, ValidateToken, GenerateTemporaryToken (write-through: Postgres + Redis)
 - internal/protocol/apptypes.go — AppID constants, ODSA operation constants
 - internal/protocol/statuscodes.go — all status enums
 - internal/protocol/request.go — EntitlementRequest struct with validator tags
 - internal/protocol/response.go — response types
 - internal/protocol/json_builder.go — port from src/protocol/jsonBuilder.ts
 - internal/protocol/xml_builder.go — port from src/protocol/xmlBuilder.ts
 - Tests for builders

 Validation: JSON/XML responses match TypeScript output for same inputs.

 ---
 Phase 6: Service Handlers

 Goal: All 12 telecom service handlers.

 Files: All internal/services/*.go — port from src/services/*.ts

 Each handler is a pure function: (status, provStatus, tcStatus, configData, odsaContext) → *ApplicationConfig

 The router in builder.go maps appId → handler via a map, queries DB for entitlement status, calls handler, formats response.

 Validation: Port responseBuilder.test.ts, odsa.test.ts, extendedServices.test.ts.

 ---
 Phase 7: HTTP Server + Middleware + Routes

 Goal: Wire everything into working HTTP servers for both ECS and mock-hss.

 Files:
 - internal/server/server.go — chi router, middleware stack, graceful shutdown
 - internal/server/routes.go — POST/GET /entitlement (4-path routing from src/server/routes/entitlement.ts), GET /health
 - internal/server/middleware.go — requestParser, userAgentParser, versionCheck, errorHandler
 - internal/server/audit.go — audit log middleware
 - internal/observability/logger.go, tracing.go, metrics.go
 - cmd/ecs/main.go — dependency construction, server start, signal handling
 - cmd/mock-hss/main.go — POST /vectors, POST /resync, GET /health

 Route handler 4-path logic (preserved from TypeScript):
 1. eap_relay present → try re-auth session, then full auth RT2
 2. token present → try re-auth state (initiate re-auth), then validate ODSA temp token
 3. Neither → EAP-AKA RT1 (requires IMSI)

 Validation: Full integration tests — EAP-AKA flow, fast re-auth, token lifecycle, ODSA operations.

 ---
 Phase 8: Docker + CI/CD + Infrastructure

 Goal: Containerize, update CI/CD, adjust Terraform.

 Files:
 - Dockerfile.ecs — multi-stage Go build → distroless
 - Dockerfile.mock-hss — multi-stage Go build → distroless
 - docker-compose.yml — updated build contexts, reduced memory limits
 - Makefile — build, test, lint, seed, docker targets
 - .github/workflows/ci.yml — go test, go vet, golangci-lint, govulncheck
 - Terraform: update Cloud Run image refs, reduce memory (1Gi→512Mi ECS, 1Gi→256Mi HSS), increase concurrency (80→500)

 ---
 Key Go Design Decisions

 Error handling: Sentinel errors (var ErrSubscriberNotFound = errors.New(...)) + wrapped errors (fmt.Errorf("fetch vectors: %w", err)). Route handler is the error boundary.

 Context propagation: Every I/O function takes context.Context as first param. Request context flows through r.Context(). Timeouts via context.WithTimeout.

 Dependency injection: Struct-based, not globals. Server struct holds db, redis, logger, hssClient. Each cmd/main.go constructs the dependency graph. Testable via interfaces.

 Buffer handling: Buffer → []byte. Buffer.from(hex,'hex') → hex.DecodeString(). buf.readUInt16BE() → binary.BigEndian.Uint16(). crypto.timingSafeEqual() → subtle.ConstantTimeCompare(). Buffer zeroing via
 clear(buf).

 Concurrency: Each HTTP request is a goroutine (automatic). EAP identifier counter uses atomic.AddUint32. pgxpool and go-redis are concurrency-safe.

 ---
 Verification Plan

 1. Unit tests — All crypto, codec, key derivation, encryption, service handler tests ported from TypeScript
 2. Golden file tests — Capture base64 EAP packet outputs from TypeScript, verify Go produces identical bytes
 3. Integration tests — Full EAP-AKA handshake, fast re-auth, ODSA operations against Docker services
 4. MILENAGE validation — 3GPP TS 35.207 official test vectors (cryptographic correctness proof)
 5. docker compose up — Both services start, health checks pass, full auth flow works end-to-end

 ---
 Existing TypeScript Code

 The TypeScript source (src/, mock-hss/src/, tests/) will be kept as reference during development and removed once the Go implementation is fully validated and passing all tests.
╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌