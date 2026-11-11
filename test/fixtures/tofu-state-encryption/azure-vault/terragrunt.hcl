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
    key_provider   = "azure_vault"
    vault_uri      = get_env("TG_AZURE_TEST_VAULT_URI")
    vault_key_name = get_env("TG_AZURE_TEST_VAULT_KEY_NAME")
    key_length     = 32
  }
}

generate "outputs" {
  path      = "outputs.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
output "encryption_test" {
  value = "azure-vault-encryption"
}
EOF
}