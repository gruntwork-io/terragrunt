# No error and remove file because the existing file has terragrunt signature.
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
