resource "google_compute_network" "vpc" {
  name                    = "${var.environment}-entitlements-vpc"
  project                 = var.project_id
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "subnet" {
  name          = "${var.environment}-entitlements-subnet"
  project       = var.project_id
  region        = var.region
  network       = google_compute_network.vpc.id
  ip_cidr_range = "10.0.0.0/20"

  private_ip_google_access = true
}

# Private Services Access for Cloud SQL / Redis private IPs.
# Explicitly pinned to 10.64.0.0/16 to avoid collision with the
# subnet (10.0.0.0/20). Without an explicit address, Google's
# auto-allocator often picks 10.0.0.0/16 which overlaps the subnet.
resource "google_compute_global_address" "private_ip_range" {
  name          = "${var.environment}-entitlements-private-ip"
  project       = var.project_id
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  address       = "10.64.0.0"
  prefix_length = 16
  network       = google_compute_network.vpc.id
}

resource "google_service_networking_connection" "private_vpc_connection" {
  network                 = google_compute_network.vpc.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.private_ip_range.name]
}
# Firewall: allow internal traffic
resource "google_compute_firewall" "allow_internal" {
  name    = "${var.environment}-entitlements-allow-internal"
  project = var.project_id
  network = google_compute_network.vpc.name

  allow {
    protocol = "tcp"
    ports    = ["0-65535"]
  }

  allow {
    protocol = "udp"
    ports    = ["0-65535"]
  }

  allow {
    protocol = "icmp"
  }

  # Only trust traffic from our own ranges, not all of 10.0.0.0/8
  source_ranges = [
    "10.0.0.0/20",  # entitlements-subnet (Cloud Run, etc.)
    "10.64.0.0/16", # PSA range (Cloud SQL, Redis)
  ]
}
