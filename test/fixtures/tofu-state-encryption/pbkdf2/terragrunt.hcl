# Test PBKDF2 encryption with local state
remote_state {
  backend = "local"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }

  config = {
    path = "${get_terragrunt_dir()}/${path_relative_to_include()}/terraform.tfstate"
  }

  encryption = {
    key_provider = "pbkdf2"
    passphrase = "randompassphrase123456"
  }
}
