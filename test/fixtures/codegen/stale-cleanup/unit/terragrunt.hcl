terraform {
  source = "./base-module"
}

generate "providers" {
  path      = "tgen_providers.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
# Placeholder generated provider config for the stale-cleanup test.
EOF
}
