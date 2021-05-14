terraform {
  source = "git::ssh://git@github.com/gruntwork-io/terragrunt.git//test/fixture-source-map/modules/vpc?ref=yori-terragrunt-source-map"
}

inputs = {
  name = "terragrunt"
}
