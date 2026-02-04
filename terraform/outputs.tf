output "ecs_url" {
  description = "URL of the entitlement server Cloud Run service"
  value       = module.cloud_run.ecs_url
}

output "mock_hss_url" {
  description = "URL of the mock HSS Cloud Run service (internal)"
  value       = module.cloud_run.mock_hss_url
}

output "database_instance" {
  description = "Cloud SQL instance name"
  value       = module.database.instance_name
}

output "redis_host" {
  description = "Memorystore Redis host"
  value       = module.redis.host
}

output "artifact_registry_repo" {
  description = "Artifact Registry repository URL"
  value       = module.artifact_registry.repository_url
}

output "ecs_service_account" {
  description = "ECS Cloud Run service account"
  value       = module.iam.ecs_service_account_email
}

output "mock_hss_service_account" {
  description = "Mock HSS Cloud Run service account"
  value       = module.iam.mock_hss_service_account_email
}
