# Phase 8: GCP Infrastructure & CI/CD — Detailed Implementation Plan

## Overview

Phase 8 takes the entitlement server from a local Docker Compose setup to a production-ready deployment on Google Cloud Platform. This covers:

- **Application changes** to make the code Cloud Run-ready (migrations, Cloud KMS, port conventions)
- **Terraform infrastructure** for all GCP resources (networking, database, redis, KMS, secrets, IAM, Cloud Run, WAF)
- **CI/CD pipeline** via Cloud Build for automated build-test-deploy on push to main

---

## Part A: Application Changes (Pre-Infrastructure)

### 1. Generate Drizzle Migrations

Run `npx drizzle-kit generate` to produce migration SQL files in `drizzle/`. These get committed to the repository and executed during deployment (not at container startup).

**Why not at startup?** Cloud Run can spin up multiple instances simultaneously. Running migrations at startup creates race conditions where two instances try to apply the same migration concurrently. Instead, migrations run as a dedicated Cloud Build step before deployment.

### 2. Create Migration Runner Script — `src/db/migrate.ts`

```typescript
// Runs drizzle-kit migrate programmatically
// Called as a Cloud Build step: npx tsx src/db/migrate.ts
```

- Connects to the database using `DATABASE_URL`
- Applies pending migrations from `drizzle/`
- Exits with code 0 on success, 1 on failure
- Logs which migrations were applied

### 3. Implement `CloudKmsKeyManager` — `mock-hss/src/kms.ts`

Add a `CloudKmsKeyManager` class alongside the existing `LocalKeyManager`:

- **New dependency**: `@google-cloud/kms` in `mock-hss/package.json`
- **Class**: `CloudKmsKeyManager` implements the existing `KeyManager` interface
- **`unwrapDek(wrappedDek)`**: Calls `client.decrypt()` with the KMS key resource name
- **Constructor**: Takes `projectId`, `locationId`, `keyRingId`, `keyId`
- **Selection logic** in `mock-hss/src/index.ts`:
  - If `LOCAL_KEK_HEX` is set → use `LocalKeyManager` (dev/test)
  - Otherwise → use `CloudKmsKeyManager` (production with Cloud KMS)
- No changes to `encryptWithDek`/`decryptWithDek` (they already use AES-256-GCM correctly)

### 4. Update Dockerfiles

**`Dockerfile` (main ECS)**:
- Change `EXPOSE 8443` → `EXPOSE 8080` (Cloud Run convention: port 8080)

**`mock-hss/Dockerfile`**:
- No port change needed (stays on 3001, Cloud Run will map via PORT env)

### 5. Update `src/config/index.ts`

- Change default PORT from `8443` to `8080`
- Add `gcpProjectId` config field (used by OpenTelemetry trace exporter in Phase 9)
- Change default `DB_POOL_SIZE` from `5` to `2` (better for serverless — each Cloud Run instance should use fewer connections)

### 6. Update `mock-hss/src/config.ts`

- Make `LOCAL_KEK_HEX` optional (no longer `required()`)
- Add Cloud KMS env vars: `GCP_PROJECT_ID`, `KMS_KEY_RING`, `KMS_KEY_NAME`, `KMS_LOCATION`
- These are only read when `LOCAL_KEK_HEX` is not set

---

## Part B: Terraform Infrastructure

### Directory Structure

```
terraform/
├── main.tf                        # Provider config, enabled APIs
├── variables.tf                   # All input variables
├── outputs.tf                     # URLs, connection strings, service accounts
├── terraform.tfvars.example       # Example variable values
├── modules/
│   ├── networking/                # VPC, subnets, VPC connector, firewall
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   └── outputs.tf
│   ├── database/                  # Cloud SQL PostgreSQL + database + user
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   └── outputs.tf
│   ├── redis/                     # Memorystore Redis
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   └── outputs.tf
│   ├── kms/                       # KMS keyring + crypto key
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   └── outputs.tf
│   ├── secrets/                   # Secret Manager secrets
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   └── outputs.tf
│   ├── artifact-registry/         # Container image repository
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   └── outputs.tf
│   ├── cloud-run/                 # Both ECS and mock-hss services
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   └── outputs.tf
│   ├── cloud-armor/               # WAF security policy
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   └── outputs.tf
│   └── iam/                       # Service accounts + role bindings
│       ├── main.tf
│       ├── variables.tf
│       └── outputs.tf
```

### Module Details

#### `networking/`
- **VPC** with custom subnet (10.0.0.0/20) in chosen region
- **Serverless VPC Connector** for Cloud Run → private Cloud SQL/Redis access
- **Private Services Access** for Cloud SQL private IP
- **Firewall rules**: allow internal traffic only

#### `database/`
- **Cloud SQL PostgreSQL 16** instance (`db-f1-micro` for staging, HA optional via variable)
- **Private IP only** (no public IP — accessed via VPC connector)
- Database `entitlements`, user `ecs`
- Password generated via `random_password`, stored in Secret Manager
- Automated backups enabled
- SSL enforcement

#### `redis/`
- **Memorystore Redis 7** (BASIC tier, 1GB for staging)
- Connected to VPC
- AUTH enabled with generated password stored in Secret Manager

#### `kms/`
- **Keyring** `entitlement-keys` in region
- **CryptoKey** `ki-kek` for envelope encryption of subscriber Ki values
- Rotation period: 90 days
- IAM: only `mock-hss-runner` service account gets `roles/cloudkms.cryptoKeyDecrypter`

#### `secrets/`
- `database-url` — constructed from Cloud SQL outputs
- `redis-url` — constructed from Memorystore outputs
- `local-kek-hex` — only for staging (production uses Cloud KMS exclusively)
- Secret versions created automatically
- IAM: Cloud Run service accounts get `roles/secretmanager.secretAccessor`

#### `artifact-registry/`
- Docker repository in region
- Cleanup policy: keep last 10 images

#### `cloud-run/`

Two services deployed:

**entitlement-server (ECS)**:
| Setting | Value |
|---------|-------|
| Port | 8080 |
| Min instances | 0 |
| Max instances | 10 |
| CPU | 1 |
| Memory | 512Mi |
| Ingress | All (public, fronted by load balancer) |
| VPC connector | Yes (for private DB/Redis access) |
| Startup probe | GET /health, period 5s, failure threshold 3 |
| Liveness probe | GET /health, period 30s |
| Service account | `ecs-runner@project.iam` |

Env vars from Secret Manager: `DATABASE_URL`, `REDIS_URL`
Plain env vars: `HSS_URL` (internal mock-hss URL), `PORT`, `NODE_ENV=production`, operator config, token TTLs

**mock-hss**:
| Setting | Value |
|---------|-------|
| Port | 3001 |
| Min instances | 0 |
| Max instances | 3 |
| Ingress | Internal only (no public access) |
| VPC connector | Yes (for Cloud SQL + KMS access) |
| Startup probe | GET /health |
| Service account | `mock-hss-runner@project.iam` (has KMS decrypt) |

#### `cloud-armor/`
- Security policy attached to load balancer
- Rate limiting: 100 req/s per IP
- SQL injection protection
- XSS protection
- Geo-restriction: optional (configurable via variable)

#### `iam/`
- **`ecs-runner`**: Cloud SQL Client, Secret Manager Accessor, Cloud Trace Agent
- **`mock-hss-runner`**: Cloud SQL Client, Secret Manager Accessor, KMS CryptoKey Decrypter, Cloud Trace Agent
- **`cloud-build`**: Cloud Run Admin, Artifact Registry Writer, Secret Manager Accessor

---

## Part C: CI/CD Pipeline

### `cloudbuild.yaml`

Triggered on push to `main` branch. Steps:

1. **Install dependencies** — `npm ci` for both ECS and mock-hss
2. **TypeScript compile** — `npm run build` for both
3. **Run unit tests** — `npm run test:unit`
4. **Build Docker images** — both ECS and mock-hss, tagged with commit SHA + `latest`
5. **Push images** to Artifact Registry
6. **Run database migrations** — connect to Cloud SQL via Cloud SQL Proxy
7. **Run seed data** — staging only
8. **Deploy mock-hss** to Cloud Run (must be up before ECS)
9. **Deploy ECS** to Cloud Run
10. **Smoke test** — curl health endpoint of deployed ECS

### Cloud Build Trigger (Terraform-managed)

- **Source**: GitHub repository connection
- **Trigger**: push to `main`
- **Config**: `cloudbuild.yaml`
- **Substitutions**: `_REGION`, `_PROJECT_ID`, `_ARTIFACT_REPO`

### Integration Tests in CI

- Steps 1–3 run in Cloud Build with sidecar postgres + redis (using Cloud Build's docker support)
- Alternative: integration tests run against the deployed staging environment as a post-deploy step

---

## Part D: Files Summary

### New Files (~30)

| File | Purpose |
|------|---------|
| `src/db/migrate.ts` | Migration runner script |
| `terraform/main.tf` | Provider, project, enabled APIs |
| `terraform/variables.tf` | All input variables |
| `terraform/outputs.tf` | URLs, connection strings, service accounts |
| `terraform/terraform.tfvars.example` | Example variable values |
| `terraform/modules/networking/{main,variables,outputs}.tf` | VPC, subnets, VPC connector |
| `terraform/modules/database/{main,variables,outputs}.tf` | Cloud SQL PostgreSQL |
| `terraform/modules/redis/{main,variables,outputs}.tf` | Memorystore Redis |
| `terraform/modules/kms/{main,variables,outputs}.tf` | KMS keyring + crypto key |
| `terraform/modules/secrets/{main,variables,outputs}.tf` | Secret Manager secrets |
| `terraform/modules/artifact-registry/{main,variables,outputs}.tf` | Container image repository |
| `terraform/modules/cloud-run/{main,variables,outputs}.tf` | Cloud Run services |
| `terraform/modules/cloud-armor/{main,variables,outputs}.tf` | WAF policy |
| `terraform/modules/iam/{main,variables,outputs}.tf` | Service accounts + roles |
| `cloudbuild.yaml` | CI/CD pipeline definition |
| `implementation_plan_phase_8.md` | This document |

### Modified Files (~5)

| File | Change |
|------|--------|
| `mock-hss/src/kms.ts` | Add `CloudKmsKeyManager` class |
| `mock-hss/src/config.ts` | Make `LOCAL_KEK_HEX` optional, add KMS env vars |
| `mock-hss/src/index.ts` | Key manager selection logic |
| `mock-hss/package.json` | Add `@google-cloud/kms` dependency |
| `Dockerfile` | Port 8443 → 8080 |
| `src/config/index.ts` | Default port 8080, pool size 2, add `gcpProjectId` |
| `.gitignore` | Add terraform state files, `.tfvars` |
| `implementation_plan.md` | Reorder phases 8/9/10 |

### Generated Files

| File | How |
|------|-----|
| `drizzle/XXXX_migration.sql` | `npx drizzle-kit generate` |

---

## Implementation Order

1. **Update plan docs** — reorder phases in `implementation_plan.md`, create this document
2. **Application changes** (Part A) — can be tested locally
3. **Terraform modules** (Part B) — bottom-up: networking → database → redis → kms → secrets → artifact-registry → iam → cloud-run → cloud-armor
4. **CI/CD pipeline** (Part C) — `cloudbuild.yaml` + trigger
5. **Initial deployment** — `terraform apply`, first image push, migrations, seed, verify

---

## Verification Checklist

1. `npm run build` — TypeScript compiles (both ECS and mock-hss)
2. `npm run test:unit` — all unit tests pass
3. `terraform plan` — no errors, shows expected resources
4. `terraform apply` — all resources created successfully
5. Push images → Cloud Build deploys to Cloud Run
6. `curl https://<ecs-url>/health` → `{ "status": "ok" }`
7. `curl -X POST https://<ecs-url>/entitlement` with test IMSI → 401 + EAP challenge
8. Integration tests pass against staging environment
9. Cloud Logging shows structured Pino logs
10. Cloud Trace shows request spans

---

## Exit Criteria

- [ ] All application changes compile and unit tests pass
- [ ] Terraform plan shows all expected resources with no errors
- [ ] Cloud Build pipeline definition is complete and valid
- [ ] Migration runner script works against local database
- [ ] CloudKmsKeyManager compiles and LocalKeyManager still works as before
- [ ] Docker images build successfully with updated port configuration
