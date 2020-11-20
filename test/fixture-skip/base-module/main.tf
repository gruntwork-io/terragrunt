variable "person" {}

data "template_file" "example" {
  template = "hello, ${var.person}"
}

output "example" {
  value = data.template_file.example.rendered
}
