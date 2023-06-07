terraform {
  source = "git::ssh://git@github.com/gruntwork-io/i-dont-exist.git//test/fixture-download/hello-world"
}

inputs = {
  name = "terragrunt"
}
