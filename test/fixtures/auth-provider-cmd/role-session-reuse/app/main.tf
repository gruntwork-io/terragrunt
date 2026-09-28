variable "msg" {
  type    = string
  default = ""
}

output "msg" {
  value = var.msg
}
