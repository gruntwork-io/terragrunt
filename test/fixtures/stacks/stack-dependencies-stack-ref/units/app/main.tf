variable "env" {
  type    = string
  default = ""
}

variable "vpc_id" {
  type    = string
  default = ""
}

output "app_status" {
  value = "running"
}
