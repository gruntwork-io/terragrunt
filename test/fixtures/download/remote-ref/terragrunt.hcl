inputs = {
  name = "World"
}

terraform {
  source = "git::git@github.com:gruntwork-io/terragrunt.git//test/fixtures/download/hello-world-no-remote?ref=v0.93.2"
}
