terraform {
  no_dot_terragrunt_cache = true
}

generate "generated" {
  path      = "generated.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOT
output "generated" {
  value = "generated"
}
EOT
}
