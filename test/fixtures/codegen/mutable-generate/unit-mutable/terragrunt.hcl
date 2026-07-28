generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite"
  mutable   = true
  contents  = <<EOF
provider "null" {
}
EOF
}
