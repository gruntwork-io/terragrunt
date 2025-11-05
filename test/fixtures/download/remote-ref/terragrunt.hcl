inputs = {
  name = "World"
}

terraform {
  source = "git::git@github.com:gruntwork-io/terragrunt.git//test/fixtures/download/hello-world-no-remote?ref=2ca67fe2dbd3001c4cffa66df9567ca497182cb1"
}
