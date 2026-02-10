# Phase 1: Foundation — Implementation Plan

## Overview
Create the entire project skeleton from scratch: package.json, TypeScript config, Docker Compose, Fastify app with middleware, Drizzle ORM schema, and health checks. ~27 files total.

## Implementation Steps (in dependency order)

### Step 1: Package scaffolding
**Files:** `package.json`, `tsconfig.json`, `.env`, `.env.example`

- `package.json` with all dependencies:
  - Runtime: `fastify`, `@fastify/type-provider-typebox`, `@sinclair/typebox`, `drizzle-orm`, `pg`, `ioredis`, `pino`, `@opentelemetry/sdk-node`, `@opentelemetry/instrumentation-http`, `@opentelemetry/instrumentation-pg`, `@opentelemetry/instrumentation-ioredis`, `@opentelemetry/exporter-trace-otlp-http`, `@opentelemetry/exporter-metrics-otlp-http`, `dotenv`
  - Dev: `typescript`, `@types/node`, `@types/pg`, `vitest`, `drizzle-kit`, `tsx`
  - Scripts: `dev`, `build`, `start`, `test`, `db:generate`, `db:migrate`
- `tsconfig.json`: target ES2022, module NodeNext, strict mode, outDir dist/
- `.env` with defaults from the implementation plan (DATABASE_URL, REDIS_URL, PORT, etc.)
- Run `npm install`

### Step 2: Config layer
**Files:** `src/config/index.ts`, `src/config/constants.ts`

- `src/config/index.ts`: Load environment variables with defaults, export typed `Config` object. Reads from process.env (dotenv loaded in entrypoint). Includes all env vars from the plan (PORT, DATABASE_URL, REDIS_URL, HSS_URL, token TTLs, SUPPORTED_VERSIONS, operator identity).
- `src/config/constants.ts`: AppID values, HTTP status codes used by the spec, protocol constants.

### Step 3: Logger
**File:** `src/config/logger.ts`

- Pino instance with Cloud Logging severity mapping (trace→DEBUG, info→INFO, error→ERROR, fatal→CRITICAL)
- Mixin stub for trace context correlation (populated later when OTel is wired)

### Step 4: OpenTelemetry initialization
**Files:** `src/config/tracing.ts`, `src/config/metrics.ts`

- `tracing.ts`: Initialize NodeSDK with OTLP trace exporter + auto-instrumentations (http, pg, ioredis). In dev mode, use console exporter or noop. **Must be imported before any other module** in `src/index.ts`.
- `metrics.ts`: Initialize MeterProvider with OTLP metrics exporter. Export meter instance for custom metrics in later phases. In dev, use console/noop exporter.

### Step 5: Database schema + connection
**Files:** `src/db/schema.ts`, `src/db/index.ts`, `drizzle.config.ts`

- `src/db/schema.ts`: Drizzle table definitions for all 5 tables (subscribers, devices, entitlements, tokens, audit_log) matching the SQL in the implementation plan.
- `src/db/index.ts`: Create pg Pool with `DB_POOL_SIZE` from config, export drizzle instance.
- `drizzle.config.ts`: Drizzle Kit config pointing at schema file and DATABASE_URL for migration generation.

### Step 6: Redis connection
**File:** `src/db/redis.ts`

- Create ioredis client from `REDIS_URL` config, export instance.
- Handle connection errors gracefully (log, don't crash).

### Step 7: Protocol schemas and types
**Files:** `src/protocol/requestSchemas.ts`, `src/protocol/requestTypes.ts`, `src/protocol/appIds.ts`, `src/protocol/statusCodes.ts`

- `requestSchemas.ts`: TypeBox schemas for EntitlementRequestBody (POST) and EntitlementRequestQuery (GET). Includes AppId union, OdsaOperation union, all field patterns/constraints from the plan.
- `requestTypes.ts`: `Static<typeof EntitlementRequestBody>` type export — single source of truth.
- `appIds.ts`: AppID enum/map (ap2003–ap2016) with human-readable names.
- `statusCodes.ts`: EntitlementStatus, ServiceStatus, SubscriptionResult enums.

### Step 8: Middleware (hooks)
**Files:** `src/server/middleware/requestParser.ts`, `src/server/middleware/userAgent.ts`, `src/server/middleware/versionCheck.ts`, `src/server/middleware/errorHandler.ts`

- `requestParser.ts`: `onRequest` hook — for GET requests, parse query string params into the EntitlementRequest shape. POST bodies are handled by Fastify schema validation natively.
- `userAgent.ts`: `onRequest` hook — parse `PRD-TS43/<version> (<vendor>; <model>; <type>; <OS>)` format, decorate request with parsed fields. Missing/malformed UA is logged but not rejected.
- `versionCheck.ts`: `onRequest` hook — compare `entitlement_version` against `SUPPORTED_VERSIONS`, reply 406 if unsupported.
- `errorHandler.ts`: Fastify `setErrorHandler` — centralized error responses with structured JSON bodies.

### Step 9: Fastify app + routes + health check
**Files:** `src/server/app.ts`, `src/server/routes/entitlement.ts`, `src/server/routes/health.ts`

- `app.ts`: Build Fastify instance with TypeBox type provider. Register hooks (requestParser, userAgent, versionCheck) as `onRequest`. Register error handler. Register route plugins. Add `onReady` hook to warm Postgres + Redis connections.
- `routes/entitlement.ts`: Register `GET /entitlement` and `POST /entitlement` routes with TypeBox schema validation. Handler is a placeholder that returns 501 (real logic comes in Phase 3+).
- `routes/health.ts`: `GET /health` returns 200 with `{ status: 'ok' }` (only reachable after onReady warmup succeeds).

### Step 10: Server entrypoint + Docker
**Files:** `src/index.ts`, `Dockerfile`, `docker-compose.yml`

- `src/index.ts`: Import tracing first (before all other imports), load dotenv, import app builder, start Fastify server with conditional TLS (disabled by default for Docker/Cloud Run).
- `Dockerfile`: Multi-stage build — `node:20-slim` base, install deps, build TypeScript, run with `node dist/index.js`.
- `docker-compose.yml`: Three services:
  - `postgres`: postgres:16, port 5432, with init healthcheck, entitlements DB
  - `redis`: redis:7, port 6379, with healthcheck
  - `ecs`: builds from Dockerfile, port 8443, depends_on postgres+redis healthy, env vars pointing at container hostnames

## Files Created (27 total)

```
package.json
tsconfig.json
.env
.env.example
drizzle.config.ts
Dockerfile
docker-compose.yml
src/
  index.ts
  config/
    index.ts
    constants.ts
    logger.ts
    tracing.ts
    metrics.ts
  server/
    app.ts
    routes/
      entitlement.ts
      health.ts
    middleware/
      requestParser.ts
      userAgent.ts
      versionCheck.ts
      errorHandler.ts
  protocol/
    requestSchemas.ts
    requestTypes.ts
    appIds.ts
    statusCodes.ts
  db/
    schema.ts
    index.ts
    redis.ts
```

## Verification

1. `npm install` completes without errors
2. `npm run build` compiles TypeScript successfully
3. `docker compose up` starts all 3 containers
4. `curl http://localhost:8443/health` → `200 { "status": "ok" }`
5. `curl -X POST http://localhost:8443/entitlement -H 'Content-Type: application/json' -d '{}'` → `400` with Ajv validation error (missing required fields)
6. `curl -X POST http://localhost:8443/entitlement -H 'Content-Type: application/json' -d '{"app":"ap2004","terminal_id":"123456789012345","entitlement_version":"2"}'` → `501` placeholder response (service not yet implemented)
