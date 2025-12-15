# Test openbao encryption with local state
remote_state {
  backend = "local"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }

  config = {
    path = "${get_terragrunt_dir()}/${path_relative_to_include()}/tofu.tfstate"
  }

  encryption = {
    key_provider = "openbao"
    key_name     = "__FILL_IN_OPENBAO_KEY_NAME__"
    address      = "__FILL_IN_OPENBAO_ADDRESS__"
    token        = "__FILL_IN_OPENBAO_TOKEN__"
  }
}
