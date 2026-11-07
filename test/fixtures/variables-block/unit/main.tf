variable "from_attr" {}
variable "from_root" {}
variable "from_env" {}
variable "shared" {}
variable "template" {}

variable "nulled" {
  default = "default"
}

variable "dropped" {
  default = "default"
}

output "values" {
  value = {
    from_attr = var.from_attr
    from_root = var.from_root
    from_env  = var.from_env
    shared    = var.shared
    template  = var.template
    nulled    = var.nulled == null ? "null" : var.nulled
    dropped   = var.dropped
  }
}
