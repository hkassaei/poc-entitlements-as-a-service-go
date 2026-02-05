# GCP Deployment Guide

## 1. A GCP Project

You need a GCP project with billing enabled. This is the single most important prerequisite — everything else lives inside it.

- A **project ID** (e.g., `entitlements-poc-12345`)
- Billing account attached (Cloud SQL, Memorystore, KMS, and Cloud Run all cost money)

## 2. Tools to Install Locally

| Tool | Purpose |
|------|---------|
| **`gcloud` CLI** | Authenticate to GCP, manage resources, set up Cloud Build trigger |
| **Terraform >= 1.5** | Provision all infrastructure (`terraform plan` / `terraform apply`) |
| **Docker** | Build container images (for initial manual push before CI/CD is wired up) |

## 3. Prerequisite Roles

If your account doesn't have **Owner** or **Editor** on the project, you need these fine-grained roles. A project Owner must run these commands (replace `YOUR_PROJECT_ID` and `YOUR_EMAIL`):

```bash
PROJECT_ID="YOUR_PROJECT_ID"
USER="user:YOUR_EMAIL"

# Artifact Registry — create Docker image repository
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="$USER" --role="roles/artifactregistry.admin" --condition=None

# Cloud KMS — create keyring and crypto keys for Ki encryption
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="$USER" --role="roles/cloudkms.admin" --condition=None

# VPC Access — create serverless VPC connector for Cloud Run
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="$USER" --role="roles/vpcaccess.admin" --condition=None

# Secret Manager — store database and Redis credentials
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="$USER" --role="roles/secretmanager.admin" --condition=None

# Cloud Run — deploy ECS and mock-hss services
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="$USER" --role="roles/run.admin" --condition=None

# IAM — create service accounts (ecs-runner, mock-hss-runner, cloud-build)
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="$USER" --role="roles/iam.serviceAccountAdmin" --condition=None

# IAM — assign roles to service accounts
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="$USER" --role="roles/resourcemanager.projectIamAdmin" --condition=None

# Service Networking — private IP allocation for Cloud SQL
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="$USER" --role="roles/servicenetworking.networksAdmin" --condition=None

# Redis — create Memorystore instance
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="$USER" --role="roles/redis.admin" --condition=None
```

You also need `roles/cloudsql.admin` and `roles/compute.securityAdmin` (for Cloud Armor), but these are commonly pre-assigned in enterprise GCP projects.

## 4. GCP Authentication

Run `gcloud auth login` and `gcloud auth application-default login` so both `gcloud` and Terraform can authenticate.

## 5. Information You Need to Decide

| Variable | What it is | Default |
|----------|-----------|---------|
| `project_id` | Your GCP project ID | *no default — required* |
| `region` | Where to deploy | `us-central1` |
| `db_tier` | Cloud SQL instance size | `db-f1-micro` (cheapest, fine for POC) |
| `db_ha_enabled` | High-availability for the DB | `false` (unnecessary for POC) |
| `operator_mcc` / `operator_mnc` / `operator_name` | Operator identity in responses | `001` / `01` / `TestOperator` |

## 6. GCP APIs That Terraform Enables Automatically

The `main.tf` already enables these via `google_project_service`, so you don't need to do it manually:

- Cloud Run, Cloud SQL Admin, Memorystore Redis, Cloud KMS, Secret Manager, Artifact Registry, Cloud Build, Compute Engine, VPC Access, Service Networking, Cloud Trace

## 7. GitHub Connection (for CI/CD)

The `cloudbuild.yaml` pipeline triggers on push to `main`. To set this up you need to:

- Connect your GitHub repo to Cloud Build (done in the GCP Console under Cloud Build > Triggers > "Connect Repository")
- Create a trigger pointing at `cloudbuild.yaml`

This is optional for the initial deployment — you can do the first deploy manually and wire up CI/CD after.

## 8. Deployment Order

Once you have a project and tools:

```bash
# 1. Configure Terraform variables
cp terraform/terraform.tfvars.example terraform/terraform.tfvars
# Fill in your project_id

# 2. Review what will be created
cd terraform && terraform init && terraform plan

# 3. Provision all infrastructure
# Creates: VPC, Cloud SQL, Memorystore, KMS, Secret Manager,
#          Artifact Registry, service accounts, Cloud Run services,
#          Cloud Armor WAF
terraform apply

# 4. Build & push Docker images manually for the first time
docker build -t us-central1-docker.pkg.dev/YOUR_PROJECT/entitlements/ecs:latest .
docker build -t us-central1-docker.pkg.dev/YOUR_PROJECT/entitlements/mock-hss:latest mock-hss/
docker push us-central1-docker.pkg.dev/YOUR_PROJECT/entitlements/ecs:latest
docker push us-central1-docker.pkg.dev/YOUR_PROJECT/entitlements/mock-hss:latest

# 5. Run migrations against Cloud SQL
npx tsx src/db/migrate.ts

# 6. Seed test data
npx tsx src/db/seed-entitlements.ts

# 7. Verify
curl https://<ecs-cloud-run-url>/health
```

## 9. Estimated GCP Cost (POC)

For a staging/POC setup with `db-f1-micro`, basic Redis, and `minInstances: 0`:

| Resource | Estimated Cost |
|----------|---------------|
| Cloud SQL (`db-f1-micro`) | ~$8/month |
| Memorystore (1GB basic) | ~$35/month |
| Cloud Run | Near-zero if idle (pay per request) |
| KMS | ~$0.06/month per key |
| Artifact Registry / Secret Manager | Negligible |
| **Total** | **~$40-50/month idle** |

The biggest line item is Memorystore (Redis).
