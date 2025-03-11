include "root" {
  path = find_in_parent_folders("root.hcl")
}

engine {
  source  = "github.com/gruntwork-io/terragrunt-engine-opentofu"
  version = "v0.0.16"
  type    = "rpc"
}

dependency "app1" {
  config_path = "../app1"
}

inputs = {
  app1_output = dependency.app1.outputs.value
}
