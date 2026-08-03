# Retrieve a module from the public OpenTofu registry. The version constraint
# is resolved to the newest matching release at download time.
terraform {
  source  = "tfr://registry.opentofu.org/yorinasub17/terragrunt-registry-test/null"
  version = "~> 0.0.1"
}
