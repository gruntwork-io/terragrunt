terraform {
  source = "git::ssh://git@github.com/gruntwork-io/i-dont-exist.git//test/fixtures/download/hello-world-no-remote"
}

inputs = {
  name = "terragrunt"
}
