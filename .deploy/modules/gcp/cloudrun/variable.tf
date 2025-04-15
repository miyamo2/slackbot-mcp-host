variable "app_name" {
  type    = string
  default = "mcp-host"
}

variable "image" {
  type      = string
  nullable  = false
  sensitive = true
}

variable "region" {
  type      = string
  nullable  = false
  sensitive = true
}