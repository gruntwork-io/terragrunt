include "b" {
  path = "../../first/b/terragrunt.hcl"
}

inputs = {
  c = dependency.a.outputs.z
}

locals {
  vvv = dependency.a.outputs.z

  ddd = dependency.d
}
