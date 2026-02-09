# ─────────────────────────────────────────────────────────
# ECS service account (entitlement-server)
# ─────────────────────────────────────────────────────────
resource "google_service_account" "ecs_runner" {
  account_id   = "ecs-runner"
  display_name = "Entitlement Server Runner"
  project      = var.project_id
}

# Cloud SQL Auth Proxy — connect to Postgres via Unix socket
resource "google_project_iam_member" "ecs_sql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.ecs_runner.email}"
}

# Cloud Trace — export distributed traces
resource "google_project_iam_member" "ecs_trace_agent" {
  project = var.project_id
  role    = "roles/cloudtrace.agent"
  member  = "serviceAccount:${google_service_account.ecs_runner.email}"
}

# Cloud Logging — write structured logs
resource "google_project_iam_member" "ecs_log_writer" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.ecs_runner.email}"
}

# Cloud Monitoring — export custom metrics
resource "google_project_iam_member" "ecs_metric_writer" {
  project = var.project_id
  role    = "roles/monitoring.metricWriter"
  member  = "serviceAccount:${google_service_account.ecs_runner.email}"
}

# Secret Manager — read DATABASE_URL, REDIS_URL at container startup
resource "google_project_iam_member" "ecs_secret_accessor" {
  project = var.project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${google_service_account.ecs_runner.email}"
}

# ─────────────────────────────────────────────────────────
# Mock HSS service account
# ─────────────────────────────────────────────────────────
resource "google_service_account" "mock_hss_runner" {
  account_id   = "mock-hss-runner"
  display_name = "Mock HSS Runner"
  project      = var.project_id
}

# Cloud SQL Auth Proxy — read encrypted Ki from Postgres
resource "google_project_iam_member" "mock_hss_sql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.mock_hss_runner.email}"
}

# Cloud Trace — export distributed traces
resource "google_project_iam_member" "mock_hss_trace_agent" {
  project = var.project_id
  role    = "roles/cloudtrace.agent"
  member  = "serviceAccount:${google_service_account.mock_hss_runner.email}"
}

# Cloud Logging — write structured logs
resource "google_project_iam_member" "mock_hss_log_writer" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.mock_hss_runner.email}"
}

# Cloud Monitoring — export custom metrics
resource "google_project_iam_member" "mock_hss_metric_writer" {
  project = var.project_id
  role    = "roles/monitoring.metricWriter"
  member  = "serviceAccount:${google_service_account.mock_hss_runner.email}"
}

# Secret Manager — read DATABASE_URL at container startup
resource "google_project_iam_member" "mock_hss_secret_accessor" {
  project = var.project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${google_service_account.mock_hss_runner.email}"
}

# ─────────────────────────────────────────────────────────
# Cloud Build service account
# ─────────────────────────────────────────────────────────
resource "google_service_account" "cloud_build" {
  account_id   = "cloud-build-deployer"
  display_name = "Cloud Build Deployer"
  project      = var.project_id
}

# Deploy Cloud Run services
resource "google_project_iam_member" "cloud_build_run_admin" {
  project = var.project_id
  role    = "roles/run.admin"
  member  = "serviceAccount:${google_service_account.cloud_build.email}"
}

# Push images to Artifact Registry
resource "google_project_iam_member" "cloud_build_ar_writer" {
  project = var.project_id
  role    = "roles/artifactregistry.writer"
  member  = "serviceAccount:${google_service_account.cloud_build.email}"
}

# Read secrets (DATABASE_URL for migrations)
resource "google_project_iam_member" "cloud_build_secret_accessor" {
  project = var.project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${google_service_account.cloud_build.email}"
}

# Act as Cloud Run service accounts during deployment
resource "google_project_iam_member" "cloud_build_sa_user" {
  project = var.project_id
  role    = "roles/iam.serviceAccountUser"
  member  = "serviceAccount:${google_service_account.cloud_build.email}"
}

# Connect to Cloud SQL for migrations
resource "google_project_iam_member" "cloud_build_sql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.cloud_build.email}"
}
