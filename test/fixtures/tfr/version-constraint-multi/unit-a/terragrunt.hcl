# One of several units sharing a module source and version constraint, so a
# single run resolves the constraint once and reuses the pin for every unit.
terraform {
  source  = "tfr://registry.opentofu.org/yorinasub17/terragrunt-registry-test/null"
  version = "~> 0.0.1"
}
