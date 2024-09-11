generate "backend" {
  path        = "backend.tf"
  disable     = true
  if_disabled = "skip"
  if_exists   = "skip"
  contents    = <<EOF
terraform {
  backend "local" {
    path = "foo.tfstate"
  }
}
EOF
}
