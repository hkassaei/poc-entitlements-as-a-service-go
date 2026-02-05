variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "environment" {
  description = "Environment name (e.g. dev, staging, prod) — used to namespace shared resources like secrets"
  type        = string
  default     = "dev"
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

variable "db_tier" {
  description = "Cloud SQL machine tier"
  type        = string
  default     = "db-f1-micro"
}

variable "db_ha_enabled" {
  description = "Enable high availability for Cloud SQL"
  type        = bool
  default     = false
}

variable "ecs_image" {
  description = "Docker image for the entitlement server"
  type        = string
  default     = "us-central1-docker.pkg.dev/PROJECT/entitlements/ecs:latest"
}

variable "mock_hss_image" {
  description = "Docker image for the mock HSS"
  type        = string
  default     = "us-central1-docker.pkg.dev/PROJECT/entitlements/mock-hss:latest"
}

variable "operator_mcc" {
  description = "Mobile Country Code"
  type        = string
  default     = "001"
}

variable "operator_mnc" {
  description = "Mobile Network Code"
  type        = string
  default     = "01"
}

variable "operator_name" {
  description = "Operator display name"
  type        = string
  default     = "TestOperator"
}

variable "domain" {
  description = "Domain name for Google-managed SSL certificate (omit for IP-only access)"
  type        = string
  default     = null
}
