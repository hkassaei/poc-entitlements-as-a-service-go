resource "google_redis_instance" "cache" {
  name               = "${var.environment}-entitlements-redis"
  project            = var.project_id
  region             = var.region
  tier               = "BASIC"
  memory_size_gb     = var.memory_size_gb
  redis_version      = "REDIS_7_0"
  authorized_network = var.network_id
  auth_enabled       = true

  redis_configs = {
    maxmemory-policy = "allkeys-lru"
  }
}
