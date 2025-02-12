# Test GCP KMS encryption with local state
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
    key_provider       = "gcp_kms"
    kms_encryption_key = "__FILL_IN_KMS_KEY_ID__"
    key_length         = 1024
  }
}
