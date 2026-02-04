output "ecs_service_account_email" {
  value = google_service_account.ecs_runner.email
}

output "mock_hss_service_account_email" {
  value = google_service_account.mock_hss_runner.email
}

output "cloud_build_service_account_email" {
  value = google_service_account.cloud_build.email
}
