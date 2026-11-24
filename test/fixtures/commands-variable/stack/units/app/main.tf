variable "vars" {
  type = list(string)
}

output "vars" {
  value = var.vars
}
