inputs = {
  name = "World"
}

terraform {
  source = "github.com/gruntwork-io/terragrunt.git?ref=test%2Fdo-no-delete//test/fixture-download/relative"
}
