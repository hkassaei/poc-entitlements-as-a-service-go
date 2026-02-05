variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "cloud_run_service_name" {
  description = "Name of the Cloud Run ECS service to route traffic to"
  type        = string
}

variable "security_policy_id" {
  description = "Cloud Armor security policy ID to attach to the backend service"
  type        = string
}

variable "domain" {
  description = "Domain name for Google-managed SSL certificate (omit for IP-only access)"
  type        = string
  default     = null
}
