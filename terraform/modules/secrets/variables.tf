variable "project_id" {
  type = string
}

variable "environment" {
  description = "Environment name used to namespace secret IDs (e.g. dev, staging, prod)"
  type        = string
}

variable "region" {
  type = string
}

variable "database_url" {
  type      = string
  sensitive = true
}

variable "redis_url" {
  type      = string
  sensitive = true
}

variable "ecs_service_account_email" {
  type = string
}

variable "mock_hss_service_account_email" {
  type = string
}
