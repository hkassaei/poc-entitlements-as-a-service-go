output "lb_ip_address" {
  description = "The internal IP address of the Load Balancer"
  value       = google_compute_forwarding_rule.default.ip_address
}