output "external_ip" {
  description = "Global static external IP address"
  value       = google_compute_global_address.default.address
}

output "https_url" {
  description = "Public HTTPS URL for the entitlement server"
  value       = var.domain != null ? "https://${var.domain}" : "https://${google_compute_global_address.default.address}"
}
