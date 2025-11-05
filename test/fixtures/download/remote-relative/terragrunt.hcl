inputs = {
  name = "World"
}

terraform {
  source = "github.com/gruntwork-io/terragrunt.git//test/fixtures/download/relative?ref=2ca67fe2dbd3001c4cffa66df9567ca497182cb1"
}
