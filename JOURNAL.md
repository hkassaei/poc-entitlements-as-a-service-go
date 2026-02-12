# Project Memory

## Architecture
- Telecom entitlement server (GSMA TS.43) with EAP-AKA auth over HTTP
- Fastify + TypeBox + Drizzle ORM + PostgreSQL + Redis
- 12 service handlers (ap2003-ap2016), routed via `responseBuilder.ts` switch
- Service handlers are pure functions: `(status, provStatus, tcStatus, configData?, odsaContext?) → ApplicationConfig`
- `extraParams: Record<string, string>` is the extension mechanism for service-specific response fields

## Key Patterns
- **Builder pattern** for service handlers (simple vs operation-aware)
- **Operation sub-routing** via `odsaContext.operation` for ODSA-style services
- **EAP-AKA fast re-auth** (RFC 4187 §5.1) replaces opaque token rotation — re-auth identities stored in Redis, cryptographic challenge/response with AES-128-CBC encrypted inner attributes
- **Idempotent seeds** using `ON CONFLICT ... DO UPDATE`
- Vitest `globalSetup` in `tests/integration/setup.ts` auto-seeds DB before integration tests
- **Split vitest configs**: `vitest.config.ts` (base, no DB) for unit tests, `vitest.integration.config.ts` (with globalSetup) for integration tests — critical for CI where unit test job has no database

## AI-Specific Gotchas
- **Never hardcode dependency versions from memory** — my training cutoff means version numbers are ~9 months stale. Always use `npm install <package>@latest` or check the registry. This project launched with OTel v1, TypeBox v0.34, Pino v9, Vitest v3 — all outdated from day one because I wrote package.json from memory instead of querying npm. Dependabot immediately opened 22 PRs.

## Gotchas
- `seed-entitlements.ts` exports `seedEntitlements()` — don't make it top-level-only again
- Request schema (`requestSchemas.ts`) validates `app` against a union of known app IDs — new services need a new literal there
- The "no entitlement" integration test uses Bob + ap2005 (genuinely missing) — don't use made-up app IDs
- Integration tests depend on Docker (postgres, redis, mock-hss) being up
- 4 pre-existing ODSA integration test failures were caused by unseeded DB — fixed in globalSetup
- HSS integration test timeouts were caused by `config` singleton loading before tests set `HSS_URL` — default was `http://mock-hss:3001` (Docker DNS, unreachable from host). Fixed by changing default to `http://localhost:3001`; Docker Compose sets the env var explicitly.
- `tokenService.test.ts` was in `tests/unit/` but connects to PostgreSQL+Redis — moved to `tests/integration/`. Any test that imports `db` or `redis` is an integration test.
- **No Drizzle migration files in repo** — `drizzle/` folder was never committed. CI must use `drizzle-kit push --force` (schema-first) not `drizzle-kit migrate` (migration-first). Push syncs schema directly to DB without needing SQL files.

## Load Balancer Terraform Learnings
- **Cloud Run ingress enum mismatch** — GCP docs/Console say `INTERNAL_AND_GCLB`, but the Terraform provider validates against `INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER`. Always check the provider's enum, not the GCP docs.
- **HTTPS proxy with empty `ssl_certificates` is broken** — won't serve TLS at all. Use a self-signed cert via `tls` provider as fallback when no domain is configured.
- **Self-managed certs need `name_prefix` + `create_before_destroy`** — GCP cert names are immutable; rotation requires create-before-destroy lifecycle.
- **`EXTERNAL_MANAGED` load balancing scheme** — required for modern ALB features (Cloud Armor integration, advanced routing). Don't use plain `EXTERNAL`.
- **Module-level `depends_on` causes dependency cycles** — when a module contains `google_compute_global_address` and depends_on a chain that includes another module with the same resource type (e.g. networking's VPC peering address), Terraform creates transitive cycles. Use implicit ordering via variable references instead.

## Cloud Run Terraform Learnings
- **Direct VPC Egress over VPC Connector** — connectors spin up proxy VMs (extra latency, cost, throughput limits). Use `network_interfaces` instead.
- **Cloud SQL Auth Proxy needs explicit volume mount** — `volumes.cloud_sql_instance` + `volume_mounts` at `/cloudsql/`, and `DATABASE_URL` must use Unix socket (`?host=/cloudsql/<connection_name>`), not `host:port`
- **`min_instance_count = 1`** for latency-sensitive services — cold start chains (ECS → mock-hss) stack up to 5-10s
- **512Mi is too low** for Node.js + Postgres + Redis + OTel — use 1Gi minimum to avoid GC thrash and OOM
- **`cpu_idle = false`** in the `resources` block — CPU throttling to zero between requests kills connection pool keep-alives. Use the native Terraform field, NOT the `run.googleapis.com/cpu-throttling` annotation (Terraform may silently ignore annotations when native fields exist)
- **`startup_cpu_boost = true`** — extra CPU during cold start for faster Node.js init and schema compilation
- **Internal Cloud Run services need explicit invoker IAM** — same project/VPC doesn't grant access. ECS service account needs `roles/run.invoker` on mock-hss, otherwise 403
- **Service accounts need logging + monitoring roles** — `logging.logWriter` and `monitoring.metricWriter` are easy to forget; without them logs/metrics silently fail
- **Enterprise GCP IAM gotchas** — org policies may block `user:` bindings (require `group:` instead), and conditional IAM policies require `--condition=None` flag on `gcloud` commands

## CI/CD Pipeline Learnings
- **ESLint `projectService: false` for test files** — tests aren't in `tsconfig.json` (rootDir is `src/`). Using `allowDefaultProject` with `**` globs is forbidden by typescript-eslint. Instead, disable `projectService` for `tests/**/*.ts` and turn off type-aware rules like `consistent-type-imports` for those files.
- **ESLint `import type` auto-detection** — `consistent-type-imports` rule catches `import { Foo }` when `Foo` is only used as a type. Fastify's `TypeBoxTypeProvider` is type-only in `.withTypeProvider<T>()` calls.
- **GitHub Actions service containers** need health checks in `options` (not `healthcheck` like docker-compose). Format: `--health-cmd "..." --health-interval 5s --health-timeout 5s --health-retries 5`
- **CodeQL requires repo-level enablement** — the action alone isn't enough. Must enable "Code scanning" in Settings > Code security and analysis first. Otherwise the analyze step fails with "Code scanning is not enabled for this repository".
- **CodeQL v3 deprecated Dec 2026** — use `github/codeql-action/*@v4`
- **Trivy finds base image CVEs** (glibc, zlib in `node:20-slim`) that have no fix available. Set `exit-code: "0"` to report without blocking, or use `--ignore-unfixed`.
- **Checkov `soft_fail: true`** initially — lets you see all findings without blocking PRs. Triage and create `.checkov.yml` skip rules before switching to hard-fail.
- **Dependabot creates PRs immediately** after config is pushed — group minor/patch updates together (`groups: { minor-and-patch: { update-types: [minor, patch] } }`) to avoid PR flood.
- **`terraform fmt -recursive`** — Terraform files drift from canonical formatting over time. Run `fmt` before committing or add it as a CI gate.
- See [cicd-learnings.md](cicd-learnings.md) for detailed notes.

## Go Re-implementation
- **Stack**: chi router + pgx/v5 raw SQL + go-redis/v9 + log/slog + stdlib crypto
- **No GORM**: 10 fixed queries with heavy bytea/jsonb — raw SQL with pgx gives full control, no reflection overhead
- **Protocol-identical**: same wire-format EAP packets, same Redis key patterns, same API contract, same DB schema as TypeScript
- **Struct-based DI**: no globals, no DI framework — `cmd/main.go` constructs the full dependency graph
- **Context propagation**: every I/O function takes `context.Context` as first param, flowing from `r.Context()`
- **Docker**: multi-stage builds → `gcr.io/distroless/static-debian12:nonroot` (minimal attack surface, ~15MB images)
- **CI**: `go build`, `go test -race`, `go vet`, `gofmt -l`, `golangci-lint`, `govulncheck`

### Go-Specific Gotchas
- **`CreateKeyManager` returns 2 values** — `KeyManager` + `error`. Easy to forget the error return when calling from `main.go`
- **Go exported field naming**: `SqnMs` not `SQNMS` — Go convention is CamelCase even for acronyms in the middle of a name
- **Pointer types for optional DB fields**: `*string`, `*int` instead of TypeScript's optional chaining
- **`gofmt` formatting drift**: 7 files needed formatting fixes after initial write — always run `gofmt -s -w .` and add `gofmt -l` check to CI
- **Buffer translation**: `binary.BigEndian.Uint16()` replaces `buf.readUInt16BE()`, `hex.DecodeString()` replaces `Buffer.from(hex,'hex')`
- **`defer crypto.ZeroSlice(dek)`** — cleaner than TypeScript try/finally for zeroing secrets
- **`encoding/hex` needed in mock-hss main.go** — `cfg.LocalKEKHex` is a string, `CreateKeyManager` expects `[]byte`, must decode hex first

### Go File Locations
- ECS entry point: `cmd/ecs/main.go`
- Mock HSS entry point: `cmd/mock-hss/main.go`
- Config + constants: `internal/config/`
- MILENAGE + envelope encryption + KMS: `internal/crypto/`
- EAP-AKA protocol (codec, keys, encryption, orchestrator, re-auth, sessions): `internal/eapaka/`
- Database (pgx, Redis, models, queries, migrate, seed): `internal/db/`
- Token service: `internal/token/service.go`
- TS.43 response builders: `internal/protocol/`
- 12 service handlers: `internal/services/`
- HTTP server + middleware: `internal/server/`
- Schema DDL: `sql/schema.sql`
- Go Docker configs: `Dockerfile.ecs`, `Dockerfile.mock-hss`, `docker-compose.go.yml`
- Go CI: `.github/workflows/ci.yml`, `.github/workflows/security.yml`

## Production TODO
- **KMS KeyRing: switch to static name + `prevent_destroy = true`** — dev/staging uses random suffix for easy destroy+recreate, but production must use a fixed name (`entitlement-keys`) with `prevent_destroy` on both the KeyRing and CryptoKey. A random Terraform glitch must never orphan the encryption key that protects the subscriber Ki database.

## EAP-AKA Re-auth Architecture
- **Flow**: Full auth → re-auth identity issued → fast re-auth via AKA-Reauthentication (subtype 13)
- **Re-auth state** (`reauthStore.ts`): long-lived Redis hash (`reauth:{id}`), stores MK/K_aut/K_encr/counter, TTL = fastAuthTokenTtlSeconds
- **Re-auth session** (`reauthSession.ts`): short-lived Redis hash (`reauth_session:{id}`), 90s TTL for challenge/response correlation
- **Encryption** (`eapEncryption.ts`): AES-128-CBC with AT_PADDING, no auto-padding (RFC 4187 §10.12)
- **Key derivation**: `deriveReauthKeys()` — K_aut/K_encr persist, only MSK'/EMSK' re-derived
- **Counter-too-small**: client sends AT_COUNTER_TOO_SMALL → server falls back to full auth
- **ODSA temporary tokens** still use DB-backed `generateToken()`/`validateToken()` — only the main auth flow uses re-auth identities
- **GET /entitlement**: read-only, checks re-auth state first then falls back to ODSA tokens

## File Locations
- Service handlers: `src/services/`
- Response routing: `src/protocol/responseBuilder.ts`
- Request validation schemas: `src/protocol/requestSchemas.ts`
- Seed data: `src/db/seed-entitlements.ts`
- Integration test setup: `tests/integration/setup.ts`
- Vitest config: `vitest.config.ts`
- Terraform modules: `terraform/modules/` (networking, database, redis, kms, secrets, artifact-registry, cloud-run, cloud-armor, iam, load-balancer)
- CI/CD pipeline: `cloudbuild.yaml` (CD), `.github/workflows/ci.yml` (CI), `.github/workflows/security.yml` (security)
- ESLint configs: `eslint.config.js`, `mock-hss/eslint.config.js`
- Vitest integration config: `vitest.integration.config.ts`
- Dependabot: `.github/dependabot.yml`
- Redis client config: `src/db/redis.ts`
- Re-auth orchestrator: `src/auth/eapReauth.ts`
- Re-auth state store: `src/auth/reauthStore.ts`
- Re-auth session store: `src/auth/reauthSession.ts`
- EAP encryption: `src/auth/eapEncryption.ts`
