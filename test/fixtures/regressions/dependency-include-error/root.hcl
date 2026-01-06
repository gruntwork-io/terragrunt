# Root config that reads layer.hcl via read_terragrunt_config
# This tests that read_terragrunt_config properly suppresses diagnostics

terraform {
  source = "."
}

locals {
  # This read_terragrunt_config call should have diagnostics suppressed
  layer_config = try(read_terragrunt_config(find_in_parent_folders("layer.hcl")), { locals = {} })
}

inputs = {
  root_value = "from_root"
}
