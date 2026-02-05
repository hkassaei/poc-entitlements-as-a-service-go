resource "google_storage_bucket" "terraform_state" {
  name          = var.state_bucket_name
  project       = var.project_id
  location      = var.region
  force_destroy = false

  public_access_prevention = "enforced"
  uniform_bucket_level_access = true

  versioning {
    enabled = true
  }
}
