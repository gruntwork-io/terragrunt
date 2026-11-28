module "child" {
  source = "./child"
}

output "ok" {
  value = module.child.ok
}
