terraform {
  source = "git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/stacks/self-include/module?ref=main"
}

inputs = {
  data = values.data
}