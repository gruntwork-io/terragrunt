locals {
  dependencies = read_terragrunt_config("dependencies.hcl")
}

inputs = {
  my_input = local.dependencies.dependency.dep.outputs.some_output
}
