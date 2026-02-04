variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "vpc_connector_id" {
  type = string
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
