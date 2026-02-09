# --- Serverless NEG (Regional) ---

resource "google_compute_region_network_endpoint_group" "ecs" {
  name                  = "entitlement-server-neg"
  project               = var.project_id
  region                = var.region
  network_endpoint_type = "SERVERLESS"

  cloud_run {
    service = var.cloud_run_service_name
  }
}

# --- Backend Service (Regional, Internal) ---

resource "google_compute_region_backend_service" "ecs" {
  name                  = "entitlement-server-backend"
  project               = var.project_id
  region                = var.region
  protocol              = "HTTP" # Cloud Run terminates TLS, internal traffic can be HTTP
  load_balancing_scheme = "INTERNAL_MANAGED"

  backend {
    group = google_compute_region_network_endpoint_group.ecs.id
  }
}

# --- URL Map (Regional) ---

resource "google_compute_region_url_map" "default" {
  name            = "entitlement-server-url-map"
  project         = var.project_id
  region          = var.region
  default_service = google_compute_region_backend_service.ecs.id
}

# --- HTTP Proxy (Regional) ---

resource "google_compute_region_target_http_proxy" "default" {
  name    = "entitlement-server-http-proxy"
  project = var.project_id
  region  = var.region
  url_map = google_compute_region_url_map.default.id
}

# --- Forwarding Rule (Regional, Internal IP) ---

resource "google_compute_forwarding_rule" "default" {
  name                  = "entitlement-server-ilb"
  project               = var.project_id
  region                = var.region
  ip_protocol           = "TCP"
  load_balancing_scheme = "INTERNAL_MANAGED"
  port_range            = "80"
  target                = google_compute_region_target_http_proxy.default.id
  network               = var.vpc_network_id
  subnetwork            = var.vpc_subnetwork_id
  network_tier          = "PREMIUM"
}