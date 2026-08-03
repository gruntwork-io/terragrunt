# The ?version= query accepts an exact version only; a constraint there is
# rejected with a typed error pointing at the terraform block's version
# attribute instead.
terraform {
  source = "tfr://registry.opentofu.org/yorinasub17/terragrunt-registry-test/null?version=~>0.0.1"
}
