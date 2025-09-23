remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    bucket       = "terragrunt-to-terralith-tfstate-2025-09-24-2359"
    key          = "${path_relative_to_include()}/tofu.tfstate"
    region       = "us-east-1"
    encrypt      = true
    use_lockfile = true
  }
}

generate "providers" {
  path      = "providers.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
provider "aws" {
  region = "us-east-1"
}
EOF
}
