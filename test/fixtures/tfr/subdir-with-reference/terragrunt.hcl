# Retrieve a module from the public terraform registry to use with terragrunt
terraform {
  source = "tfr://registry.terraform.io/yorinasub17/terragrunt-registry-test/null//modules/two?version=0.0.2"
}
