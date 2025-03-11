include "root" {
  path = find_in_parent_folders("root.hcl")
}

engine {
  source  = "github.com/gruntwork-io/terragrunt-engine-opentofu"
  version = "v0.0.16"
  type    = "rpc"
}
