variable "name" {
  type        = string
  description = "Specify a name"
}

output "test" {
  value = "${module.hello.hello}, ${var.name}"
}

module "hello" {
  source = "./hello"
}

module "remote" {
  source = "github.com/gruntwork-io/terragrunt.git//test/fixture-download/hello-world?ref=v0.9.9"
  name   = var.name
}
