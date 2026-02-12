# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Entitlements-as-a-Service is a telecommunications entitlement server that authenticates mobile devices using SIM card hardware tokens via the EAP-AKA (Extensible Authentication Protocol - Authentication and Key Agreement) protocol over HTTP/HTTPS. The system follows the GSMA TS.43 standard.

Implemented in Go. Module path: `github.com/hkassaei/poc-entitlements-as-a-service-go`.

## Architecture

Two services, both in this repo:

- **ECS** (`cmd/ecs/main.go`) — Internet-facing entitlement server. Handles EAP-AKA authentication, token management, and serves entitlement configurations for 12 telecom services.
- **Mock HSS** (`cmd/mock-hss/main.go`) — Internal-only service holding subscriber secrets (Ki/OP). Generates authentication vectors via MILENAGE.

Key packages under `internal/`:
- `config/` — Environment config + EAP-AKA constants
- `crypto/` — MILENAGE (3GPP TS 35.206), AES-256-GCM envelope encryption, KMS abstraction
- `eapaka/` — EAP-AKA protocol: binary codec, key derivation, encryption, orchestrator, fast re-auth, Redis session stores, idempotency cache
- `db/` — pgx/v5 raw SQL queries, go-redis client, models, schema migration, seed
- `token/` — Token generation/validation with Redis write-through cache
- `protocol/` — TS.43 JSON + XML response builders, status codes, request types
- `services/` — 12 telecom service handlers (VoWiFi, VoLTE, SMSoIP, ODSA, etc.)
- `server/` — chi router, middleware, route handlers, audit logging

## Requirements

Protocol Compliance (GSMA TS.43 / EAP-AKA protocol as specified in RFC 4187). All the implementation in this project shall be in compliance with the specification. Do not re-invent any solution if it already is standardized in the spec.

## Build & Test Commands

```bash
go build ./...              # Build all packages
go test ./... -count=1      # Run all tests
go test ./... -race         # Run tests with race detector
go vet ./...                # Static analysis
gofmt -s -w .               # Format all files
make build                  # Build both binaries to bin/
make test                   # Run tests
make docker-up              # Start all services via Docker Compose
```

## Technology Choices

- **chi** for HTTP routing (lightweight, close to stdlib)
- **pgx/v5** with raw SQL (no ORM — 10 fixed queries, heavy bytea/jsonb)
- **go-redis/v9** for Redis (sessions, re-auth state, caching)
- **log/slog** for structured JSON logging
- **crypto/*** stdlib for all cryptography
- **testify** for test assertions

## Project Knowledge

See [JOURNAL.md](JOURNAL.md) for accumulated architectural decisions, gotchas, and lessons learned. Consult it before making changes to avoid repeating past mistakes. When you learn something new (bugs, gotchas, architectural decisions, lessons), update JOURNAL.md to keep it current.

## Coding Best Practices

- Do not keep any dead code around. If you find dead code from previous iterations that is not exercised any more, refactor and clean them up. Always run all unit and integration tests after removing dead code.
- Every I/O function takes `context.Context` as its first parameter.
- Use `subtle.ConstantTimeCompare` for all cryptographic comparisons.
- Use `defer crypto.ZeroSlice()` immediately after obtaining any secret key material.
- Pointer types (`*string`, `*int`) for nullable database columns.
- Run `gofmt -s -w .` before committing — CI enforces formatting.
