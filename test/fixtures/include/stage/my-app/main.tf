terraform {
  backend "s3" {}
}

# Create an arbitrary local resource
data "template_file" "test" {
  template = "Hello, I am a template."
}

variable "reflect" {}
output "reflect" {
  value = var.reflect
}
