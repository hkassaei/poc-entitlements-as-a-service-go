# Database URL secret
# User-managed replication across two Canadian regions for:
# - Resilience: survives a single-region Secret Manager outage
# - Data sovereignty: secrets never leave Canada
resource "google_secret_manager_secret" "database_url" {
  secret_id = "${var.environment}-database-url"
  project   = var.project_id

  replication {
    user_managed {
      replicas {
        location = "northamerica-northeast1" # Montreal
      }
      replicas {
        location = "northamerica-northeast2" # Toronto
      }
    }
  }
}

resource "google_secret_manager_secret_version" "database_url" {
  secret      = google_secret_manager_secret.database_url.id
  secret_data = var.database_url
}

# Redis URL secret
resource "google_secret_manager_secret" "redis_url" {
  secret_id = "${var.environment}-redis-url"
  project   = var.project_id

  replication {
    user_managed {
      replicas {
        location = "northamerica-northeast1" # Montreal
      }
      replicas {
        location = "northamerica-northeast2" # Toronto
      }
    }
  }
}

resource "google_secret_manager_secret_version" "redis_url" {
  secret      = google_secret_manager_secret.redis_url.id
  secret_data = var.redis_url
}

# IAM: ECS service account can access secrets
resource "google_secret_manager_secret_iam_member" "ecs_database_url" {
  secret_id = google_secret_manager_secret.database_url.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${var.ecs_service_account_email}"
}

resource "google_secret_manager_secret_iam_member" "ecs_redis_url" {
  secret_id = google_secret_manager_secret.redis_url.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${var.ecs_service_account_email}"
}

# IAM: Mock HSS service account can access secrets
resource "google_secret_manager_secret_iam_member" "mock_hss_database_url" {
  secret_id = google_secret_manager_secret.database_url.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${var.mock_hss_service_account_email}"
}
