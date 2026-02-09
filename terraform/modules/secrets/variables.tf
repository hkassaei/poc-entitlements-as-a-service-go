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
