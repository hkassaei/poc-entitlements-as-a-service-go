resource "google_compute_security_policy" "entitlements_waf" {
  name        = "entitlements-waf"
  description = "WAF for Entitlement Server - OWASP Top 10 Protection"

  # -----------------------------------------------------------
  # 1. Default Rule: Allow traffic
  # -----------------------------------------------------------
  rule {
    action   = "allow"
    priority = "2147483647"
    match {
      versioned_expr = "SRC_IPS_V1"
      config {
        src_ip_ranges = ["*"]
      }
    }
    description = "Default allow"
  }

  # -----------------------------------------------------------
  # 2. Rate Limiting (DDoS Protection)
  # -----------------------------------------------------------
  rule {
    action   = "rate_based_ban"
    priority = "1000"
    match {
      versioned_expr = "SRC_IPS_V1"
      config {
        src_ip_ranges = ["*"]
      }
    }
    rate_limit_options {
      conform_action = "allow"
      exceed_action  = "deny(429)"

      # Tuned: 100/sec is extremely high for a single user.
      # 2000 requests in 5 minutes is a safer "real user" ceiling.
      rate_limit_threshold {
        count        = 2000
        interval_sec = 300
      }
      ban_duration_sec = 300 # Ban for 5 minutes
    }
    description = "Rate limit: 2000 req / 5 min per IP"
  }

  # -----------------------------------------------------------
  # 3. OWASP Core Rule Set (SQLi, XSS, RCE, LFI)
  # -----------------------------------------------------------

  # Grouping critical vulnerability protections
  # NOTE: We use 'preview = true' for the first week to detect false positives

  rule {
    action   = "deny(403)"
    priority = "2000"
    preview  = true # <--- CRITICAL FOR DAY 1: Log only, don't block yet
    match {
      expr {
        expression = <<EOT
          evaluatePreconfiguredExpr('sqli-v33-stable') || 
          evaluatePreconfiguredExpr('xss-v33-stable') ||
          evaluatePreconfiguredExpr('lfi-v33-stable') ||
          evaluatePreconfiguredExpr('rce-v33-stable') ||
          evaluatePreconfiguredExpr('scannerdetection-v33-stable')
        EOT
      }
    }
    description = "OWASP Top 10: SQLi, XSS, LFI, RCE, Scanners (Preview Mode)"
  }

  # -----------------------------------------------------------
  # 4. Protocol Attack Protection (HTTP Smuggling, etc.)
  # -----------------------------------------------------------
  rule {
    action   = "deny(403)"
    priority = "2001"
    match {
      expr {
        expression = "evaluatePreconfiguredExpr('protocolattack-v33-stable')"
      }
    }
    description = "Block invalid HTTP protocol usage"
  }
}