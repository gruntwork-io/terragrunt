variable "msg" {
  type    = string
  default = "I can echo back msg value"
}

locals {
  msg = length(var.msg) > 0 ? var.msg : "Please provide a value for msg var"
}

output "echo" {
  value = local.msg
}
