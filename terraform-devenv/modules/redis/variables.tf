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

variable "network_id" {
  type = string
}

variable "memory_size_gb" {
  type    = number
  default = 1
}
