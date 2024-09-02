terraform {
  source = "git::ssh://git@github.com/gruntwork-io/another-dont-exist.git//fixture-source-map/modules/vpc?ref=master"
}

inputs = {
  name = "terragrunt"
}
