terraform {
  source = "../modules/test-module"
}

dependencies {
  paths = ["../other"]
}

dependency "other" {
  config_path = "../other"
}

generate "provider_test" {
  path      = "provider_test.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
# Generated provider config with token from dependency
# Token: ${dependency.other.outputs.secrets.test_provider_token}
EOF
}

inputs = {
  # This should work even in broken version
  token_via_input = dependency.other.outputs.secrets.test_provider_token
}
