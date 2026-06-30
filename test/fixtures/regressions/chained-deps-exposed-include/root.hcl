dependency "ancestor" {
  config_path = "${get_terragrunt_dir()}/../ancestor-dependency"

  mock_outputs = {
    bucket = "mock-bucket"
  }
  mock_outputs_merge_with_state = true
}

locals {
  common = "shared-value"
}

remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    path = "${get_terragrunt_dir()}/${dependency.ancestor.outputs.bucket}.tfstate"
  }
}
