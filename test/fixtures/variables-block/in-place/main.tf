variable "from_tfvar" {}

variable "from_json" {}

output "values" {
  value = {
    from_tfvar = var.from_tfvar
    from_json  = var.from_json
  }
}
