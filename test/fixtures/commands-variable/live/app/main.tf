variable "vars" {
  type = list(string)
}

variable "root_locking" {
  type = list(string)
}

output "vars" {
  value = var.vars
}

output "root_locking" {
  value = var.root_locking
}
