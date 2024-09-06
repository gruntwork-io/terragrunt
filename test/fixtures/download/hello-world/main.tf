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
  source = "github.com/gruntwork-io/terragrunt.git//test/fixtures/download/hello-world?ref=5a58053a6a08bac1c7b184e21f536a83cd48a3fa"
  name   = var.name
}
