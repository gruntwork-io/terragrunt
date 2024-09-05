inputs = {
  name = "World"
}

terraform {
  source = "git::git@github.com:gruntwork-io/terragrunt.git//test/fixture-download/hello-world?ref=fixture/test"
}
