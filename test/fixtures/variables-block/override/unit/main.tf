variable "root_only" {}
variable "from_attr" {}
variable "from_block" {}

output "values" {
  value = {
    root_only  = var.root_only
    from_attr  = var.from_attr
    from_block = var.from_block
  }
}
