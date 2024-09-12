generate "backend" {
  path        = "backend.tf"
  disable     = true
  if_disabled = "remove"
  if_exists   = "overwrite"
  contents    = <<EOF
terraform {
  backend "local" {
    path = "foo.tfstate"
  }
}
EOF
}
