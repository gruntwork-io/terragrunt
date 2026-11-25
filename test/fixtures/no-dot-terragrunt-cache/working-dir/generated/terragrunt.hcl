terraform {
  no_dot_terragrunt_cache = true
}

generate "main" {
  path      = "main.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOT
output "value" {
  value = "generated"
}
EOT
}
