module "foo" {
  source = "../hello-world"
  name   = var.name
}

variable "name" {
  description = "Specify a name"
}

output "test" {
  value = module.foo.test
}