generate "alpha" {
  path        = "alpha.tf"
  if_exists   = "overwrite_terragrunt"
  if_disabled = "remove"
  disable     = values.provider != "alpha"
  contents    = <<EOF
terraform {
  required_providers {
    null = {
      source  = "hashicorp/null"
      version = ">= 3.0"
    }
  }
}
EOF
}

generate "beta" {
  path        = "beta.tf"
  if_exists   = "overwrite_terragrunt"
  if_disabled = "remove"
  disable     = values.provider != "beta"
  contents    = <<EOF
terraform {
  required_providers {
    random = {
      source  = "hashicorp/random"
      version = ">= 3.0"
    }
  }
}
EOF
}
