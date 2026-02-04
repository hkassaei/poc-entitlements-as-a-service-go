resource "google_artifact_registry_repository" "docker" {
  repository_id = "entitlements"
  project       = var.project_id
  location      = var.region
  format        = "DOCKER"
  description   = "Container images for entitlements-as-a-service"

  cleanup_policies {
    id     = "keep-last-10"
    action = "KEEP"

    most_recent_versions {
      keep_count = 10
    }
  }
}
