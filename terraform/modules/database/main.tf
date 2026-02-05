resource "random_password" "db_password" {
  length  = 32
  special = false
}

# Cloud SQL reserves instance names for ~1 week after deletion.
# The random suffix ensures a fresh name on every terraform apply cycle,
# avoiding "instance name already in use" errors after destroy+recreate.
resource "random_id" "db_suffix" {
  byte_length = 4
}

resource "google_sql_database_instance" "postgres" {
  name             = "${var.environment}-entitlements-db-${random_id.db_suffix.hex}"
  project          = var.project_id
  region           = var.region
  database_version = "POSTGRES_16"


  settings {
    tier              = var.tier
    availability_type = (var.environment == "prod" || var.environment == "staging") ? "REGIONAL" : "ZONAL"
    disk_size         = 10
    disk_autoresize   = true

    ip_configuration {
      ipv4_enabled                                  = false
      private_network                               = var.network_id
      enable_private_path_for_google_cloud_services = true
      require_ssl                                   = true
    }

    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = true
      start_time                     = "03:00"
    }

    database_flags {
      name  = "cloudsql.iam_authentication"
      value = "on"
    }
  }

  deletion_protection = true
}

resource "google_sql_database" "entitlements" {
  name     = "entitlements"
  instance = google_sql_database_instance.postgres.name
  project  = var.project_id
}

resource "google_sql_user" "ecs" {
  name     = "ecs"
  instance = google_sql_database_instance.postgres.name
  project  = var.project_id
  password = random_password.db_password.result
}
