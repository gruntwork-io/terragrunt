locals {
  version = "main"
}

unit "app1" {
  source = "git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/stacks/self-include/unit?ref=${local.version}"
  path   = "app1"
  values = {
    data = "example-data"
  }
}


