# CI/CD Pipeline Implementation Plan

## Context

The project has zero CI/CD automation in GitHub. There's an existing `cloudbuild.yaml` for GCP deployment, but no PR checks, no security scanning, no linting, and no automated quality gates. Every commit to main is untested at the CI level. The user wants a full-fledged pipeline with quality and security gates, extensible for future additions.

**User's confirmed choices:**
- **Hybrid**: GitHub Actions for CI (build, lint, test, security), Cloud Build for CD (deploy to GCP)
- **PR-based workflow**: feature branch → PR → checks pass → merge → Cloud Build deploys
- **Full security suite**: CodeQL for SAST, Gitleaks for secret detection, Trivy for container scanning

---

## Critical Issue: Vitest globalSetup

Current `vitest.config.ts` has `globalSetup: ['./tests/integration/setup.ts']` which connects to PostgreSQL to seed data before ALL tests — including unit tests. In CI, the unit test job has no database, so it would fail.

**Fix**: Split into two configs:
- `vitest.config.ts` — base config (no globalSetup), used by `npm run test:unit`
- `vitest.integration.config.ts` — extends base, adds globalSetup, used by `npm run test:integration`

---

## Implementation Steps

### Step 1: ESLint Setup (both projects)

Neither the ECS nor mock-hss project has a linter. Add ESLint v9 flat config with TypeScript support.

**Files:**
- `eslint.config.js` (ECS root)
- `mock-hss/eslint.config.js`

**Config approach**: Use `@eslint/js` recommended + `typescript-eslint` recommended. Type-aware linting with `parserOptions.projectService: true`. Rules: no unused vars (error, allow `_` prefix), no explicit any (warn), consistent type imports.

**package.json changes** (both):
- Add devDependencies: `eslint`, `@eslint/js`, `typescript-eslint`
- Add script: `"lint": "eslint src/"` (ECS also lints `tests/`)

### Step 2: Vitest Config Split

**Files:**
- `vitest.config.ts` — remove `globalSetup`, keep as base config
- `vitest.integration.config.ts` — new file, adds `globalSetup: ['./tests/integration/setup.ts']`

**package.json changes:**
- `"test:unit"` stays as `"vitest run tests/unit"` (uses base config, no DB needed)
- `"test:integration"` becomes `"vitest run tests/integration --config vitest.integration.config.ts"`
- `"test"` becomes `"vitest run"` (runs all tests, uses base config — unit tests won't need DB)

Actually, the cleaner approach: keep `"test"` pointing to the integration config (so `npm test` runs everything with DB seeding), and have `"test:unit"` explicitly use the base config:
- `"test": "vitest run --config vitest.integration.config.ts"` — full suite (needs DB)
- `"test:unit": "vitest run tests/unit"` — unit only (no DB, uses base vitest.config.ts)
- `"test:integration": "vitest run tests/integration --config vitest.integration.config.ts"` — integration only (needs DB)

### Step 3: GitHub Actions CI Workflow

**File:** `.github/workflows/ci.yml`

Triggers: `push` to `main`, `pull_request` to `main`

**Job 1: build-lint** (no services needed)
- Checkout, setup Node 20, `npm ci`
- `npm run build` (TypeScript compilation)
- `npm run lint`
- `cd mock-hss && npm ci && npm run build && npm run lint`

**Job 2: unit-tests** (no services needed, depends on build-lint)
- Checkout, setup Node 20, `npm ci`, `npm run build`
- `npm run test:unit` (uses base vitest.config.ts — no DB connection)
- `cd mock-hss && npm ci && npm test` (mock-hss unit tests, no DB needed)

**Job 3: integration-tests** (needs postgres, redis, mock-hss; depends on build-lint)
- Service containers: `postgres:16`, `redis:7`
- Checkout, setup Node 20
- `npm ci && npm run build` for both ECS and mock-hss
- Start mock-hss in background (`node mock-hss/dist/index.js &`)
- Run DB migrations: `npx tsx src/db/migrate.ts`
- `npm run test:integration`
- Environment: `DATABASE_URL`, `REDIS_URL`, `HSS_URL`, `LOCAL_KEK_HEX`

### Step 4: Terraform Validation (in CI workflow)

Add a **terraform-validate** job to `ci.yml` (no GCP credentials needed).

**Job 4: terraform-validate** (no services, no cloud credentials)
- Uses `hashicorp/setup-terraform` action (version matching `required_version >= 1.5`)
- `terraform fmt -check -recursive` — formatting gate
- `terraform init -backend=false` — skip GCS backend (no credentials), just fetch provider schemas
- `terraform validate` — syntax, type, and config errors

This catches broken Terraform on every PR without needing GCP access. A future enhancement can add `terraform plan` on main-branch pushes using Workload Identity Federation.

### Step 5: GitHub Actions Security Workflow

**File:** `.github/workflows/security.yml`

Triggers: `push` to `main`, `pull_request` to `main`, `schedule` (weekly for CodeQL)

**Job 1: codeql** (GitHub's SAST — application code)
- Uses `github/codeql-action/init` with `languages: javascript-typescript`
- Autobuild + analyze
- Runs on schedule too (catches new vulnerability patterns)

**Job 2: gitleaks** (secret detection)
- Uses `gitleaks/gitleaks-action@v2`
- Catches accidentally committed secrets

**Job 3: checkov** (infrastructure-as-code security — Terraform)
- Uses `bridgecrewio/checkov-action@v12`
- Scans `terraform/` directory for cloud misconfigurations
- Catches: unencrypted resources, overly permissive IAM, missing SSL, no key rotation, etc.
- Maps findings to CIS/SOC2/HIPAA benchmarks
- `--soft-fail` initially to avoid blocking PRs while we triage baseline findings, then switch to hard-fail once clean
- Can add `.checkov.yml` to skip known-acceptable findings (e.g., dev environment relaxations)

**Job 4: trivy** (container vulnerability scanning)
- Build Docker images, scan with `aquasecurity/trivy-action`
- Scans both ECS and mock-hss images
- Severity: HIGH,CRITICAL
- Only on `push` to `main` (images aren't built on PRs)

**File:** `.gitleaksignore` — allowlist known test constants:
- `LOCAL_KEK_HEX` in docker-compose.yml and test files (test-only 32-byte key)
- Mock subscriber Ki values in seed files

### Step 6: Dependabot Configuration

**File:** `.github/dependabot.yml`

Configures automated dependency management for both npm ecosystems and Terraform providers.

**Ecosystems:**
- `npm` (root `/`) — ECS dependencies, weekly schedule
- `npm` (`/mock-hss`) — Mock-HSS dependencies, weekly schedule
- `terraform` (`/terraform`) — Terraform provider versions, monthly schedule
- `github-actions` (`/`) — Keep GH Actions pinned to latest, monthly schedule

**Settings per ecosystem:**
- `open-pull-requests-limit: 10` — prevent PR flood
- `reviewers` — can be configured to auto-assign
- Labels: `dependencies` for npm, `infrastructure` for Terraform
- Group minor/patch updates together to reduce PR noise (`groups: { minor-and-patch: { update-types: [minor, patch] } }`)

This gives us:
- Automated CVE remediation (Dependabot security updates are enabled by default)
- Scheduled version bumps that run through our full CI pipeline before merge
- Terraform provider updates (catches GCP provider breaking changes early)
- GitHub Actions version updates (security-critical — pinned action versions can have CVEs too)

### Step 7: GitHub Actions Deploy Tracking Workflow

**File:** `.github/workflows/deploy.yml`

Triggers: `push` to `main` (after CI passes)

This is lightweight — it creates a GitHub deployment record pointing to the Cloud Build execution. The actual deployment is handled by Cloud Build trigger (already configured via Terraform).

**Jobs:**
- Create GitHub deployment with environment `dev`
- Poll Cloud Build status (optional, can be a simple notification)

### Step 8: Branch Protection Rules (documented, applied manually or via gh CLI)

- Require PR before merging to `main`
- Required status checks: `build-lint`, `unit-tests`, `integration-tests`, `terraform-validate`, `codeql`, `gitleaks`, `checkov`
- Require branches to be up-to-date before merging
- No direct pushes to `main`

---

## Files Summary

**New files (8):**
- `.github/workflows/ci.yml` — Build, lint, unit tests, integration tests, Terraform validate
- `.github/workflows/security.yml` — CodeQL, Gitleaks, Checkov, Trivy
- `.github/dependabot.yml` — Automated dependency updates (npm, Terraform, GH Actions)
- `eslint.config.js` — ECS ESLint flat config
- `mock-hss/eslint.config.js` — Mock-HSS ESLint flat config
- `vitest.integration.config.ts` — Integration test vitest config with globalSetup
- `.gitleaksignore` — Known test secret allowlist
- `.checkov.yml` — Checkov skip rules for known-acceptable findings (if needed after triage)

**Modified files (4):**
- `package.json` — Add eslint devDeps, lint script, fix test:integration script
- `mock-hss/package.json` — Add eslint devDeps, lint script
- `vitest.config.ts` — Remove globalSetup (becomes base config)
- `.github/workflows/deploy.yml` — Optional deploy tracking

---

## Verification

1. `npm run lint` — ESLint passes on ECS codebase
2. `cd mock-hss && npm run lint` — ESLint passes on mock-hss
3. `npm run build` — TypeScript still compiles
4. `npm run test:unit` — Unit tests pass WITHOUT database (critical check)
5. `docker compose up -d && npm run test:integration` — Integration tests still pass with database
6. Review GitHub Actions YAML for syntax: `actionlint` or push to branch and observe
7. Branch protection: `gh api repos/{owner}/{repo}/branches/main/protection` after setup


----
## Implementation
  ---
  CI/CD Pipeline Implementation — Complete

  Here's what was implemented:

  New Files (8)
  ┌────────────────────────────────┬────────────────────────────────────────────────────────────────────────────────────────┐
  │              File              │                                        Purpose                                         │
  ├────────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────┤
  │ .github/workflows/ci.yml       │ Build, lint, unit tests, integration tests, Terraform validate                         │
  ├────────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────┤
  │ .github/workflows/security.yml │ CodeQL SAST, Gitleaks secret detection, Checkov IaC scanning, Trivy container scanning │
  ├────────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────┤
  │ .github/dependabot.yml         │ Automated dependency updates for npm (both projects), Terraform, GitHub Actions        │
  ├────────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────┤
  │ eslint.config.js               │ ESLint v9 flat config with TypeScript support for ECS                                  │
  ├────────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────┤
  │ mock-hss/eslint.config.js      │ ESLint v9 flat config with TypeScript support for mock-hss                             │
  ├────────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────┤
  │ vitest.integration.config.ts   │ Integration test config with globalSetup (DB seeding)                                  │
  ├────────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────┤
  │ .gitleaksignore                │ Allowlist for known test secrets                                                       │
  └────────────────────────────────┴────────────────────────────────────────────────────────────────────────────────────────┘
  Modified Files (9)
  ┌──────────────────────────────────────────────┬─────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
  │                     File                     │                                                     Change                                                      │
  ├──────────────────────────────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ package.json                                 │ Added eslint devDeps, lint script, split test scripts                                                           │
  ├──────────────────────────────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ mock-hss/package.json                        │ Added eslint devDeps, lint script                                                                               │
  ├──────────────────────────────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ vitest.config.ts                             │ Removed globalSetup — now base config for unit tests                                                            │
  ├──────────────────────────────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ src/auth/eapAka.ts                           │ Removed unused HssSubscriberNotFoundError import, prefixed unused clientIp params with _                        │
  ├──────────────────────────────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ src/auth/eapReauth.ts                        │ Prefixed unused clientIp param with _                                                                           │
  ├──────────────────────────────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ src/server/app.ts                            │ Changed to import type for TypeBoxTypeProvider                                                                  │
  ├──────────────────────────────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ mock-hss/src/index.ts                        │ Changed to import type for TypeBoxTypeProvider                                                                  │
  ├──────────────────────────────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ mock-hss/src/milenage.ts                     │ Removed unused R5 constant (dead code)                                                                          │
  ├──────────────────────────────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ tests/integration/eapAka.integration.test.ts │ Removed unused buildChallengeResponse function, unused buildIdentity/deriveKeys imports, unused atRand variable │
  ├──────────────────────────────────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ 6 Terraform files                            │ Auto-formatted with terraform fmt                                                                               │
  └──────────────────────────────────────────────┴─────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
  Verification Results

  - Build: Both projects compile clean
  - Lint: 0 errors (5 acceptable no-explicit-any warnings)
  - Unit tests: 185 pass without database (critical for CI)
  - Integration tests: 25 pass with database
  - Mock-hss tests: 36 pass
  - Full suite (npm test): All 246 tests pass
  - Terraform: fmt -check and validate both pass

  CI Pipeline Architecture

  PR opened → ci.yml triggers
    ├── build-lint (TypeScript compile + ESLint for both projects)
    ├── unit-tests (185 ECS + 36 mock-hss tests, no DB needed)
    ├── integration-tests (25 tests with postgres + redis + mock-hss service containers)
    └── terraform-validate (fmt check + init + validate, no GCP creds needed)

  PR opened → security.yml triggers
    ├── codeql (GitHub SAST for JS/TS)
    ├── gitleaks (secret detection)
    └── checkov (Terraform security scanning, soft-fail initially)

  Push to main → security.yml also runs
    └── trivy (container vulnerability scanning for both Docker images)

  Weekly schedule → codeql (catches new vulnerability patterns)

  Next Steps

  1. Push to a branch and open a PR to see the workflows run
  2. Set branch protection rules after the first successful run — required checks: build-lint, unit-tests, integration-tests, terraform-validate, codeql, gitleaks, checkov
  3. Triage Checkov findings — currently soft_fail: true, switch to hard-fail once clean
  4. Populate .gitleaksignore with commit hashes if gitleaks flags existing test constants

# Key learnings and gotchas

● Here's the summary of key learnings:

###  The Biggest Gotcha: "Unit Tests" That Aren't

  tokenService.test.ts was in tests/unit/ but imported db and redis — modules that open network connections at import time. Locally it passed because Docker was running. In CI without a database, it crashed
  with ECONNREFUSED. Lesson: If a test imports anything that opens a socket, it's an integration test, full stop.

###  Vitest globalSetup Poisons Unit Tests

  The original single vitest.config.ts had globalSetup that connects to PostgreSQL — this runs before all tests including unit tests. Had to split into two configs: a bare one for unit tests (no DB), and one
  with globalSetup for integration tests.

###  Drizzle Push vs Migrate

  The project uses drizzle-kit but never committed migration files to the repo. CI can't use drizzle-kit migrate without a drizzle/ folder. Had to use drizzle-kit push --force instead, which applies the schema
   directly. Production should eventually switch to committed migrations for audit trails.

###  ESLint v9 + TypeScript: The projectService Trap

  Test files aren't in tsconfig.json (because rootDir: "src"). Using allowDefaultProject with ** globs is explicitly forbidden by typescript-eslint. The fix: disable projectService entirely for test files and
  turn off type-aware rules for them.

###  CodeQL Needs Manual Enablement

  The workflow alone isn't enough — you must enable "Code scanning" in the repo's Settings page first. Without it, the analyze step fails with a permissions error.

###  Trivy Finds Unfixable CVEs

  node:20-slim has glibc/zlib CVEs with will_not_fix status. Set exit-code: "0" to report without blocking, since there's literally no fix available upstream.

###  Dependabot Activates Instantly

  The moment you push the config, it opens PRs — it immediately created 6+ PRs for action version bumps, Terraform provider updates, and npm dependencies. Grouping minor+patch updates is essential to avoid PR
  flood.

###  Terraform Formatting Drifts Silently

  6 of 33 .tf files had formatting issues that nobody noticed. The terraform fmt -check CI gate catches this immediately.


---

# CICD Adaptations for DEVENV 

 Plan: Adapt CI/CD Pipeline for terraform-devenv + terraform

 Context

 The project has two Terraform configurations:
 - terraform/ — staging/production: external ALB, Cloud Armor, HTTPS, public-facing
 - terraform-devenv/ — devenv: internal ILB, no Cloud Armor, HTTP-only, no internet, sandboxed GCP

 The CI workflow (ci.yml) only validates terraform/. The security workflow (security.yml) Checkov only scans terraform/. The CD pipeline (cloudbuild.yaml) hardcodes cd terraform for deployment. All three need
  to support both environments.

 ---
 Step 1: ci.yml — Terraform validate becomes a matrix job

 Convert the terraform-validate job to a matrix strategy that validates both directories in parallel.

 File: .github/workflows/ci.yml (lines 160-180)

 Replace the single terraform-validate job with:
 terraform-validate:
   name: Terraform Validate (${{ matrix.dir }})
   runs-on: ubuntu-latest
   strategy:
     matrix:
       dir: [terraform, terraform-devenv]
   steps:
     - uses: actions/checkout@v6
     - uses: hashicorp/setup-terraform@v3
       with:
         terraform_version: "~1.5"
     - name: Check formatting
       run: terraform fmt -check -recursive
       working-directory: ${{ matrix.dir }}
     - name: Initialize (no backend)
       run: terraform init -backend=false
       working-directory: ${{ matrix.dir }}
     - name: Validate
       run: terraform validate
       working-directory: ${{ matrix.dir }}

 ---
 Step 2: security.yml — Checkov scans both directories

 Convert the checkov job to a matrix strategy that scans both Terraform directories.

 File: .github/workflows/security.yml (lines 47-59)

 Replace the single checkov job with:
 checkov:
   name: Infrastructure Security (${{ matrix.dir }})
   runs-on: ubuntu-latest
   strategy:
     matrix:
       dir: [terraform, terraform-devenv]
   steps:
     - uses: actions/checkout@v6
     - uses: bridgecrewio/checkov-action@v12
       with:
         directory: ${{ matrix.dir }}/
         framework: terraform
         soft_fail: true
         output_format: cli
         quiet: true

 ---
 Step 3: cloudbuild.yaml — Parameterize Terraform directory

 Add a _TERRAFORM_DIR substitution so the CD pipeline can target either environment. Default remains terraform (staging/prod). Devenv triggers override to terraform-devenv.

 File: cloudbuild.yaml

 Changes:
 1. Add substitution: _TERRAFORM_DIR: 'terraform'
 2. Step 4 (terraform-apply, line 60): change cd terraform → cd ${_TERRAFORM_DIR}

 When creating a Cloud Build trigger for devenv, override:
 - _TERRAFORM_DIR=terraform-devenv
 - _ENVIRONMENT=devenv

 ---
 Step 4: Format terraform-devenv

 Run terraform fmt -recursive on terraform-devenv/ so it passes the new CI formatting gate. The directory was created manually and may have drift.

 ---
 Files Modified

 1. .github/workflows/ci.yml — terraform-validate job → matrix with [terraform, terraform-devenv]
 2. .github/workflows/security.yml — checkov job → matrix with [terraform, terraform-devenv]
 3. cloudbuild.yaml — add _TERRAFORM_DIR substitution, use in terraform step
 4. terraform-devenv/**/*.tf — formatting fixes if needed (via terraform fmt)

 Verification

 1. terraform -chdir=terraform-devenv fmt -check -recursive — no formatting drift
 2. terraform -chdir=terraform-devenv init -backend=false && terraform -chdir=terraform-devenv validate — validates clean
 3. terraform -chdir=terraform fmt -check -recursive && terraform -chdir=terraform init -backend=false && terraform -chdir=terraform validate — original still validates
 4. YAML syntax review of ci.yml and security.yml