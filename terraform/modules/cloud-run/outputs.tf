output "ecs_url" {
  value = google_cloud_run_v2_service.ecs.uri
}

output "mock_hss_url" {
  value = google_cloud_run_v2_service.mock_hss.uri
}

output "ecs_service_name" {
  value = google_cloud_run_v2_service.ecs.name
}

output "mock_hss_service_name" {
  value = google_cloud_run_v2_service.mock_hss.name
}
