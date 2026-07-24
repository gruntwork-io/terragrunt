# No published version of this module satisfies the constraint, so resolution
# must fail with a typed no-matching-version error.
terraform {
  source  = "tfr://registry.opentofu.org/yorinasub17/terragrunt-registry-test/null"
  version = "~> 9.0"
}
