variable "from_root" {
  type    = string
  default = ""
}

resource "null_resource" "test" {}

output "result" {
  value = var.from_root
}
