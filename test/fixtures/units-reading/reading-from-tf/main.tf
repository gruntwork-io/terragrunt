variable "filename" {}
output "shared" {
  value = jsondecode(file(var.filename))
}
