locals {
  version = "stack-scanning-4111"
}

unit "app1" {
  source = "git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/stacks/self-include/units?ref=${local.version}"
  path   = "app1"
  values = {
    data = "example-data"
  }
}


