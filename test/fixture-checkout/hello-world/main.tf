# Create an arbitrary local resource
data "template_file" "test" {
  template = "Hello, ${var.name}"
}

variable "name" {
  description = "Specify a name"
}

output "test" {
  value = "${data.template_file.test.rendered}"
}