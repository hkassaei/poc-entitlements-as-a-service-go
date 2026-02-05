variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "vpc_network" {
  description = "VPC network name for Direct VPC Egress"
  type        = string
}

variable "vpc_subnetwork" {
  description = "VPC subnetwork name for Direct VPC Egress"
  type        = string
}

variable "ecs_image" {
  type = string
}

variable "mock_hss_image" {
  type = string
}

variable "ecs_service_account_email" {
  type = string
}

variable "mock_hss_service_account_email" {
  type = string
}

variable "database_url_secret_id" {
  type = string
}

variable "cloudsql_connection_name" {
  description = "Cloud SQL instance connection name (project:region:instance) for Auth Proxy"
  type        = string
}

variable "redis_url_secret_id" {
  type = string
}

variable "operator_mcc" {
  type    = string
  default = "001"
}

variable "operator_mnc" {
  type    = string
  default = "01"
}

variable "operator_name" {
  type    = string
  default = "TestOperator"
}
