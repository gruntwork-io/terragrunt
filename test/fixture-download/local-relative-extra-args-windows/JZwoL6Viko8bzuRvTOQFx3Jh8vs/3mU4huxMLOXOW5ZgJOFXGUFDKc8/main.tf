data "template_file" "test" {
  template = "${module.hello.hello}, ${var.name}"
}

variable "name" {
  description = "Specify a name"
}

output "test" {
  value = data.template_file.test.rendered
}

module "hello" {
  source = "./hello"
}

module "remote" {
  source = "github.com/gruntwork-io/terragrunt.git//test/fixture-download/hello-world?ref=v0.9.9"
  name   = var.name
}
