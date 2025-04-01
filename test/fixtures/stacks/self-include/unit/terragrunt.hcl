terraform {
  source = "git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/stacks/self-include/module?ref=stack-scanning-4111"
}

input = {
  data = values.data
}