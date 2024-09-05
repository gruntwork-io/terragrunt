inputs = {
  name = "World"
}

terraform {
  source = "github.com/gruntwork-io/terragrunt.git//test/fixtures/download/hello-world?ref=v0.9.9"
}
