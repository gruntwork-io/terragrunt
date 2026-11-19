locals {
  use_dependency = false
}

dependency "dep" {
  config_path = "../dep"

  mock_outputs = {
    constraint = ">= 1.0"
  }
}

terraform_version_constraint = local.use_dependency ? dependency.dep.outputs.constraint : ">= 1.0"
