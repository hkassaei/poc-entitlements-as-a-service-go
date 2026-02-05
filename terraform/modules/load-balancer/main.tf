# --- Static External IP ---

resource "google_compute_global_address" "default" {
  name    = "entitlement-server-ip"
  project = var.project_id
}

# --- Serverless NEG pointing to Cloud Run ---

resource "google_compute_region_network_endpoint_group" "ecs" {
  name                  = "entitlement-server-neg"
  project               = var.project_id
  region                = var.region
  network_endpoint_type = "SERVERLESS"

  cloud_run {
    service = var.cloud_run_service_name
  }
}

# --- Backend Service with Cloud Armor ---

resource "google_compute_backend_service" "ecs" {
  name                  = "entitlement-server-backend"
  project               = var.project_id
  protocol              = "HTTPS"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  security_policy       = var.security_policy_id

  backend {
    group = google_compute_region_network_endpoint_group.ecs.id
  }

  log_config {
    enable      = true
    sample_rate = 1.0
  }
}

# --- URL Map (main — routes all traffic to backend) ---

resource "google_compute_url_map" "default" {
  name            = "entitlement-server-url-map"
  project         = var.project_id
  default_service = google_compute_backend_service.ecs.id
}

# --- Google-managed SSL Certificate (production — when domain is set) ---

resource "google_compute_managed_ssl_certificate" "default" {
  count   = var.domain != null ? 1 : 0
  name    = "entitlement-server-cert"
  project = var.project_id

  managed {
    domains = [var.domain]
  }
}

# --- Self-signed SSL Certificate (dev — when no domain is set) ---

resource "tls_private_key" "self_signed" {
  count     = var.domain == null ? 1 : 0
  algorithm = "RSA"
  rsa_bits  = 2048
}

resource "tls_self_signed_cert" "self_signed" {
  count           = var.domain == null ? 1 : 0
  private_key_pem = tls_private_key.self_signed[0].private_key_pem

  validity_period_hours = 8760 # 1 year

  subject {
    common_name  = "entitlement-server.dev.internal"
    organization = "Development"
  }

  allowed_uses = [
    "key_encipherment",
    "digital_signature",
    "server_auth",
  ]
}

resource "google_compute_ssl_certificate" "self_signed" {
  count       = var.domain == null ? 1 : 0
  name_prefix = "entitlement-server-dev-"
  project     = var.project_id
  private_key = tls_private_key.self_signed[0].private_key_pem
  certificate = tls_self_signed_cert.self_signed[0].cert_pem

  lifecycle {
    create_before_destroy = true
  }
}

# --- HTTPS Proxy ---

resource "google_compute_target_https_proxy" "default" {
  name    = "entitlement-server-https-proxy"
  project = var.project_id
  url_map = google_compute_url_map.default.id

  ssl_certificates = var.domain != null ? [
    google_compute_managed_ssl_certificate.default[0].id
  ] : [
    google_compute_ssl_certificate.self_signed[0].id
  ]
}

# --- HTTPS Forwarding Rule (port 443) ---

resource "google_compute_global_forwarding_rule" "https" {
  name                  = "entitlement-server-https"
  project               = var.project_id
  ip_address            = google_compute_global_address.default.id
  port_range            = "443"
  target                = google_compute_target_https_proxy.default.id
  load_balancing_scheme = "EXTERNAL_MANAGED"
}

# --- HTTP → HTTPS Redirect ---

resource "google_compute_url_map" "http_redirect" {
  name    = "entitlement-server-http-redirect"
  project = var.project_id

  default_url_redirect {
    https_redirect         = true
    strip_query            = false
    redirect_response_code = "MOVED_PERMANENTLY_DEFAULT"
  }
}

resource "google_compute_target_http_proxy" "redirect" {
  name    = "entitlement-server-http-proxy"
  project = var.project_id
  url_map = google_compute_url_map.http_redirect.id
}

resource "google_compute_global_forwarding_rule" "http_redirect" {
  name                  = "entitlement-server-http-redirect"
  project               = var.project_id
  ip_address            = google_compute_global_address.default.id
  port_range            = "80"
  target                = google_compute_target_http_proxy.redirect.id
  load_balancing_scheme = "EXTERNAL_MANAGED"
}
