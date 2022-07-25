generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite"
  contents = <<EOF
# test
EOF
}