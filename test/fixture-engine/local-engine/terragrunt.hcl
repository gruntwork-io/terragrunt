engine {
  source  = "terragrunt-iac-engine-opentofu_v0.0.1"
  version = "v0.0.1"
  type    = "rpc"
}

inputs = {
  value = "test_input_value_from_terragrunt"
}
