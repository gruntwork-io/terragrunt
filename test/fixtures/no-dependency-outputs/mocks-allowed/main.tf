variable "x" {
  type    = string
  default = "unset"
}

output "x" {
  value = var.x
}
