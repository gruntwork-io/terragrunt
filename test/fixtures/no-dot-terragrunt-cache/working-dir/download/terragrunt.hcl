terraform {
  no_dot_terragrunt_cache = true
}

generate "download" {
  path      = "${path.download}/download.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOT
output "download" {
  value = "download"
}
EOT
}
