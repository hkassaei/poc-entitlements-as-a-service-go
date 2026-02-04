output "network_id" {
  value = google_compute_network.vpc.id
}

output "subnet_id" {
  value = google_compute_subnetwork.subnet.id
}

output "vpc_connector_id" {
  value = google_vpc_access_connector.connector.id
}

output "private_ip_range_name" {
  value = google_compute_global_address.private_ip_range.name
}
