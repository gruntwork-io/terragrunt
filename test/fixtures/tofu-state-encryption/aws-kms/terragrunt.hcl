# Test AWS KMS encryption with local state
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
    key_provider = "aws_kms"
    region       = "__FILL_IN_AWS_REGION__"
    kms_key_id   = "__FILL_IN_KMS_KEY_ID__"
    key_spec     = "AES_256"
  }
}
