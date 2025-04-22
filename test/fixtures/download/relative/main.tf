module "foo" {
  source = "../hello-world-no-remote"
  name   = var.name
}

variable "name" {
  description = "Specify a name"
}

output "test" {
  value = module.foo.test
}
