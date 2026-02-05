resource "google_artifact_registry_repository" "docker" {
  repository_id = "entitlements"
  project       = var.project_id
  location      = var.region
  format        = "DOCKER"
  description   = "Container images for entitlements-as-a-service"

  # -----------------------------------------------------------
  # 1. FIX: Immutable Tags
  # -----------------------------------------------------------
  # Prevents overwriting "v1.0" with different code. 
  # Forces developers to use new tags (v1.1, v1.2) for changes.
  docker_config {
    immutable_tags = true
  }

  # -----------------------------------------------------------
  # 2. FIX: Effective Cleanup Policy (Delete + Keep)
  # -----------------------------------------------------------
  
  # Policy A: The "Garbage Collector"
  # Attempt to delete anything older than 30 days.
  cleanup_policies {
    id     = "delete-old-images"
    action = "DELETE"
    condition {
      older_than = "2592000s" # 30 days in seconds
    }
  }

  # Policy B: The "Safety Net" (Exception to Policy A)
  # Even if an image is 100 days old, if it is one of the 
  # last 10 versions, KEEP it.
  cleanup_policies {
    id     = "keep-last-10"
    action = "KEEP"
    most_recent_versions {
      keep_count = 10
    }
  }
}