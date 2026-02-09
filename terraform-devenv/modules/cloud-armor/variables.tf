variable "geo_restriction_enabled" {
  description = "Enable geographic restriction"
  type        = bool
  default     = false
}

variable "allowed_countries" {
  description = "List of allowed country codes (ISO 3166-1 alpha-2)"
  type        = list(string)
  default     = []
}
