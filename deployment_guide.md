# GCP Deployment Guide

This project has two Terraform configurations:

| Config | Purpose | Load Balancer | Cloud Armor | TLS |
|--------|---------|---------------|-------------|-----|
| `terraform/` | Staging / Production | External ALB (global) | Yes | HTTPS |
| `terraform-devenv/` | Dev environment | Internal ILB (regional) | No | HTTP |

Both share the same module structure. Choose the one that matches your target environment.

## 1. Prerequisites

### GCP Project

You need a GCP project with billing enabled. Everything lives inside it.

### Tools

| Tool | Purpose |
|------|---------|
| **`gcloud` CLI** | Authenticate to GCP |
| **Terraform >= 1.5** | Provision all infrastructure |
| **Docker** | Build container images for the initial manual deploy |
| **Node.js 20** | Build the application and push DB schema |

### IAM Roles

If your account has **Editor** on the project, you only need two additional roles (Editor cannot create service accounts or assign IAM roles):

```bash
PROJECT_ID="your-project-id"
USER="user:your-email@company.com"

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="$USER" --role="roles/iam.serviceAccountAdmin" --condition=None

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="$USER" --role="roles/resourcemanager.projectIamAdmin" --condition=None
```

> **Enterprise GCP note:** Some org policies block `user:` bindings and require `group:` instead. If the commands above fail, ask your GCP admin to grant these roles via a group.

### GCP Authentication

```bash
gcloud auth login
gcloud auth application-default login
```

## 2. GCP APIs

Terraform enables all required APIs automatically via `google_project_service` resources:

Cloud Run, Cloud SQL Admin, Memorystore Redis, Cloud KMS, Secret Manager, Artifact Registry, Cloud Build, Compute Engine, Service Networking, Cloud Trace

The devenv config additionally enables **IAP** (Identity-Aware Proxy) for SSH access to the simulator VM.

## 3. Configuration Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `project_id` | Your GCP project ID | — | Yes |
| `state_bucket_name` | GCS bucket for Terraform state | — | Yes |
| `environment` | Environment name (namespaces resources) | `dev` | No |
| `region` | GCP region | `us-central1` | No |
| `db_tier` | Cloud SQL instance size | `db-f1-micro` | No |
| `db_ha_enabled` | High-availability for Cloud SQL | `false` | No |
| `ecs_image` | Docker image for entitlement server | placeholder | No |
| `mock_hss_image` | Docker image for mock HSS | placeholder | No |
| `operator_mcc` / `operator_mnc` / `operator_name` | Operator identity in responses | `001` / `01` / `TestOperator` | No |
| `domain` | Domain for managed SSL cert (staging/prod only) | `null` (IP-only) | No |

## 4. Deployment Steps

```bash
# 1. Create Terraform state bucket (one-time)
gcloud storage buckets create gs://YOUR_PROJECT-terraform-state \
  --location=us-central1 --uniform-bucket-level-access

# 2. Configure variables
#    For staging/prod:
cp terraform/terraform.tfvars.example terraform/terraform.tfvars
#    For devenv:
cp terraform-devenv/terraform.tfvars.example terraform-devenv/terraform.tfvars
#    Edit the file: fill in project_id and state_bucket_name

# 3. Provision infrastructure
cd terraform  # or terraform-devenv
terraform init
terraform plan     # review what will be created
terraform apply    # creates: VPC, Cloud SQL, Memorystore, KMS, Secret Manager,
                   #          Artifact Registry, service accounts, Cloud Run,
                   #          Load Balancer (+ Cloud Armor for staging/prod)

# 4. Authenticate Docker to Artifact Registry
gcloud auth configure-docker us-central1-docker.pkg.dev

# 5. Build and push container images
docker build -t us-central1-docker.pkg.dev/YOUR_PROJECT/entitlements/ecs:latest .
docker build -t us-central1-docker.pkg.dev/YOUR_PROJECT/entitlements/mock-hss:latest mock-hss/
docker push us-central1-docker.pkg.dev/YOUR_PROJECT/entitlements/ecs:latest
docker push us-central1-docker.pkg.dev/YOUR_PROJECT/entitlements/mock-hss:latest

# 6. Push database schema (no migration files — schema-first approach)
DATABASE_URL="postgresql://ecs:PASSWORD@IP:5432/entitlements" npx drizzle-kit push --force

# 7. Seed test data
DATABASE_URL="postgresql://ecs:PASSWORD@IP:5432/entitlements" npx tsx src/db/seed-entitlements.ts

# 8. Verify
curl https://<load-balancer-ip>/health     # staging/prod
curl http://<load-balancer-ip>/health      # devenv (HTTP, internal only)
```

> **Database schema note:** This project uses `drizzle-kit push --force` (schema-first), not migration files. There is no `drizzle/` folder in the repo. The push command syncs the Drizzle schema definition directly to the database.

## 5. CI/CD Pipeline

### GitHub Actions (CI)

The CI workflow (`.github/workflows/ci.yml`) runs on every push/PR to `main`:
- Build and lint (ECS + mock-hss)
- Unit tests
- Integration tests (with Postgres, Redis, and mock-hss service containers)
- Terraform validate (both `terraform/` and `terraform-devenv/` in parallel)

### Cloud Build (CD)

The `cloudbuild.yaml` pipeline handles continuous deployment:
1. Build and test
2. Build and push Docker images
3. Bootstrap Terraform state bucket
4. `terraform apply`
5. Database migrations
6. Smoke test

The pipeline uses a `_TERRAFORM_DIR` substitution (default: `terraform`). To deploy the devenv, create a Cloud Build trigger with overrides:
- `_TERRAFORM_DIR=terraform-devenv`
- `_ENVIRONMENT=devenv`

### Connecting GitHub to Cloud Build

In the GCP Console: Cloud Build > Triggers > "Connect Repository". Create a trigger pointing at `cloudbuild.yaml`. This is optional for the initial deployment.

## 6. Devenv-Specific Notes

The devenv (`terraform-devenv/`) is a sandboxed environment with:
- **Internal Load Balancer** — only reachable from within the VPC
- **No Cloud Armor** — not applicable to internal LBs
- **HTTP only** — no TLS termination
- **Simulator VM** — a jump host inside the VPC for testing. Access via:
  ```bash
  gcloud compute ssh device-simulator --tunnel-through-iap --project=YOUR_PROJECT --zone=us-central1-a
  ```

## 7. Estimated GCP Cost (POC)

For a POC setup with `db-f1-micro`, basic Redis, and `minInstances: 0`:

| Resource | Estimated Cost |
|----------|---------------|
| Cloud SQL (`db-f1-micro`) | ~$8/month |
| Memorystore (1GB basic) | ~$35/month |
| Cloud Run | Near-zero if idle (pay per request) |
| KMS | ~$0.06/month per key |
| Artifact Registry / Secret Manager | Negligible |
| Simulator VM (`e2-micro`, devenv only) | ~$5/month |
| **Total** | **~$40-50/month idle** |

The biggest line item is Memorystore (Redis).
