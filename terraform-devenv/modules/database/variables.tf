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

variable "private_ip_range_name" {
  type = string
}


variable "tier" {
  type    = string
  default = "db-f1-micro"
}

variable "ha_enabled" {
  type    = bool
  default = false
}

