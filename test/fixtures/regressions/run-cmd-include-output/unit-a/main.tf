variable "marker" {
  type    = string
  default = ""
}

output "marker_value" {
  value = var.marker
}
