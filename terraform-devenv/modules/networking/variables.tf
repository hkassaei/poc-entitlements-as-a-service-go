variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "environment" {
  description = "Environment name (e.g. dev, staging, prod)"
  type        = string
}
