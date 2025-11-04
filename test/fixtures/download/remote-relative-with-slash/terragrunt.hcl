inputs = {
  name = "World"
}

terraform {
  source = "github.com/gruntwork-io/terragrunt.git?ref=2ca67fe2dbd3001c4cffa66df9567ca497182cb1//test/fixtures/download/relative"
}
