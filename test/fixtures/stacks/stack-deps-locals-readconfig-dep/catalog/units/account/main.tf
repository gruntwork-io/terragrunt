variable "account" {
  type = string
}

variable "env" {
  type = string
}

variable "region" {
  type    = string
  default = ""
}

variable "marker" {
  type    = string
  default = ""
}

output "val" {
  value = "account-output"
}
