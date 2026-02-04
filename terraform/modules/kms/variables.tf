variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "mock_hss_service_account" {
  description = "Email of the mock-hss service account that needs decrypt permission"
  type        = string
}
