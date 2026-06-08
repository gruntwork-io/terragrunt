variable "roles" {
  type = list(string)
}

variable "env" {
  type = string
}

variable "region" {
  type    = string
  default = ""
}

variable "val" {
  type    = string
  default = "missing"
}
