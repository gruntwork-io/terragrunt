terraform {
  no_dot_terragrunt_cache = true
}

generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOT
locals {
  generated = "generated"
}
EOT
}

remote_state {
  backend = "local"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }

  config = {
    path = "state/terraform.tfstate"
  }
}
