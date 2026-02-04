# ECS service account
resource "google_service_account" "ecs_runner" {
  account_id   = "ecs-runner"
  display_name = "Entitlement Server Runner"
  project      = var.project_id
}

resource "google_project_iam_member" "ecs_sql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.ecs_runner.email}"
}

resource "google_project_iam_member" "ecs_trace_agent" {
  project = var.project_id
  role    = "roles/cloudtrace.agent"
  member  = "serviceAccount:${google_service_account.ecs_runner.email}"
}

# Mock HSS service account
resource "google_service_account" "mock_hss_runner" {
  account_id   = "mock-hss-runner"
  display_name = "Mock HSS Runner"
  project      = var.project_id
}

resource "google_project_iam_member" "mock_hss_sql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.mock_hss_runner.email}"
}

resource "google_project_iam_member" "mock_hss_trace_agent" {
  project = var.project_id
  role    = "roles/cloudtrace.agent"
  member  = "serviceAccount:${google_service_account.mock_hss_runner.email}"
}

# Cloud Build service account
resource "google_service_account" "cloud_build" {
  account_id   = "cloud-build-deployer"
  display_name = "Cloud Build Deployer"
  project      = var.project_id
}

resource "google_project_iam_member" "cloud_build_run_admin" {
  project = var.project_id
  role    = "roles/run.admin"
  member  = "serviceAccount:${google_service_account.cloud_build.email}"
}

resource "google_project_iam_member" "cloud_build_ar_writer" {
  project = var.project_id
  role    = "roles/artifactregistry.writer"
  member  = "serviceAccount:${google_service_account.cloud_build.email}"
}

resource "google_project_iam_member" "cloud_build_secret_accessor" {
  project = var.project_id
  role    = "roles/secretmanager.secretAccessor"
  member  = "serviceAccount:${google_service_account.cloud_build.email}"
}

resource "google_project_iam_member" "cloud_build_sa_user" {
  project = var.project_id
  role    = "roles/iam.serviceAccountUser"
  member  = "serviceAccount:${google_service_account.cloud_build.email}"
}
