# Error because the existing file does not have terragrunt signature.
generate "backend" {
  path        = "backend.tf"
  disable     = true
  if_disabled = "remove_terragrunt"
  if_exists   = "overwrite_terragrunt"
  contents    = <<EOF
terraform {
  backend "local" {
    path = "foo.tfstate"
  }
}
EOF
}
