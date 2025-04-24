inputs = {
  name = "World"
}

terraform {
  source = "github.com/gruntwork-io/terragrunt.git?ref=v0.77.22&depth=1//test/fixtures/download/relative"
}
