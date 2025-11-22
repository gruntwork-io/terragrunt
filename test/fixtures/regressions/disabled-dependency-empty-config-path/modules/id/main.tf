variable "prefix" {
  type    = string
  default = ""
}

variable "suffix" {
  type    = string
  default = ""
}

variable "separator" {
  type    = string
  default = "-"
}

resource "random_string" "this" {
  length  = 4
  upper   = false
  special = false
}

output "random_string" {
  value = random_string.this.result
}

output "id" {
  value = format(
    "%s%s%s",
    (var.prefix == "" ? "" : format("%s%s", trimsuffix(var.prefix, var.separator), var.separator)),
    random_string.this.result,
    (var.suffix == "" ? "" : format("%s%s", var.separator, trimprefix(var.suffix, var.separator))),
  )
}
