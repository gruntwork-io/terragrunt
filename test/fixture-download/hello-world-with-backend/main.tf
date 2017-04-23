data "template_file" "test" {
  template = "${module.hello.hello}, ${var.name}"
}

variable "name" {
  description = "Specify a name"
}

output "test" {
  value = "${data.template_file.test.rendered}"
}

terraform {
  # These settings will be filled in by Terragrunt
  backend "s3" {}
}