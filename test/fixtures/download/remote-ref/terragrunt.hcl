inputs = {
  name = "World"
}

terraform {
  source = "git::git@github.com:gruntwork-io/terragrunt.git//test/fixtures/download/hello-world?ref=fixture/test"
}
