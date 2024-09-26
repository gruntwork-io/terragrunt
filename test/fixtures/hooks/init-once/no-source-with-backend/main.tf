terraform {
  backend "s3" {}
}

data "template_file" "example" {
  template = "hello, world"
}

output "example" {
  value = data.template_file.example.rendered
}
