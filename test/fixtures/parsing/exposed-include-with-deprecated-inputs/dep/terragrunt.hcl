# Mock dependency configuration
terraform {
  source = "git::git@github.com:gruntwork-io/terragrunt.git//test/fixtures/download/hello-world"
}

inputs = {
  some_value = "test-value"
}
