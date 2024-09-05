module "mod" {
  source = "./module"
}

output "foo" {
  value = module.mod.foo
}
