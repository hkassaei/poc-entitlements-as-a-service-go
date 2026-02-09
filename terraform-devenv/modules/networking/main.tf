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

  # Critical: Allows VMs without public IPs to reach Google APIs (Artifact Registry, etc.)
  private_ip_google_access = true
}

# Proxy-Only Subnet: Required for Regional Internal Application Load Balancers
resource "google_compute_subnetwork" "proxy_subnet" {
  name          = "${var.environment}-entitlements-proxy-subnet"
  project       = var.project_id
  region        = var.region
  network       = google_compute_network.vpc.id
  ip_cidr_range = "10.129.0.0/23" # Dedicated range for Envoy proxies
  purpose       = "REGIONAL_MANAGED_PROXY"
  role          = "ACTIVE"
}

# Private Services Access for Cloud SQL / Redis private IPs.
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

# Firewall: Allow internal traffic between subnets
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

  source_ranges = [
    "10.0.0.0/20",   # Application subnet
    "10.129.0.0/23", # Proxy subnet (Load Balancer)
    "10.64.0.0/16",  # PSA range
  ]
}

# Firewall: Allow IAP (Identity-Aware Proxy) for SSH access to the Simulator VM
resource "google_compute_firewall" "allow_iap_ssh" {
  name    = "${var.environment}-entitlements-allow-iap-ssh"
  project = var.project_id
  network = google_compute_network.vpc.name

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  # Google IAP's IP range
  source_ranges = ["35.235.240.0/20"]
}