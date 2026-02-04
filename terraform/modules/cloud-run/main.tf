# Mock HSS — internal only, must be deployed before ECS
resource "google_cloud_run_v2_service" "mock_hss" {
  name     = "mock-hss"
  location = var.region
  project  = var.project_id
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"

  template {
    service_account = var.mock_hss_service_account_email

    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }

    vpc_access {
      connector = var.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    containers {
      image = var.mock_hss_image

      ports {
        container_port = 3001
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }

      env {
        name  = "PORT"
        value = "3001"
      }

      env {
        name  = "NODE_ENV"
        value = "production"
      }

      env {
        name  = "GCP_PROJECT_ID"
        value = var.project_id
      }

      env {
        name = "DATABASE_URL"
        value_source {
          secret_key_ref {
            secret  = var.database_url_secret_id
            version = "latest"
          }
        }
      }

      startup_probe {
        http_get {
          path = "/health"
          port = 3001
        }
        period_seconds    = 5
        failure_threshold = 3
      }
    }
  }
}

# Entitlement Server (ECS) — public
resource "google_cloud_run_v2_service" "ecs" {
  name     = "entitlement-server"
  location = var.region
  project  = var.project_id
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = var.ecs_service_account_email

    scaling {
      min_instance_count = 0
      max_instance_count = 10
    }

    vpc_access {
      connector = var.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    containers {
      image = var.ecs_image

      ports {
        container_port = 8080
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }

      env {
        name  = "PORT"
        value = "8080"
      }

      env {
        name  = "NODE_ENV"
        value = "production"
      }

      env {
        name  = "GCP_PROJECT_ID"
        value = var.project_id
      }

      env {
        name  = "HSS_URL"
        value = google_cloud_run_v2_service.mock_hss.uri
      }

      env {
        name  = "OPERATOR_MCC"
        value = var.operator_mcc
      }

      env {
        name  = "OPERATOR_MNC"
        value = var.operator_mnc
      }

      env {
        name  = "OPERATOR_NAME"
        value = var.operator_name
      }

      env {
        name = "DATABASE_URL"
        value_source {
          secret_key_ref {
            secret  = var.database_url_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "REDIS_URL"
        value_source {
          secret_key_ref {
            secret  = var.redis_url_secret_id
            version = "latest"
          }
        }
      }

      startup_probe {
        http_get {
          path = "/health"
          port = 8080
        }
        period_seconds    = 5
        failure_threshold = 3
      }

      liveness_probe {
        http_get {
          path = "/health"
          port = 8080
        }
        period_seconds = 30
      }
    }
  }
}

# Allow unauthenticated access to ECS (public API)
resource "google_cloud_run_v2_service_iam_member" "ecs_public" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.ecs.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}
