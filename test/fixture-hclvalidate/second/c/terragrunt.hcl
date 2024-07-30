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

// should not cause a dependency cycle
dependency iam {
  //config_path = "../iam"
}
