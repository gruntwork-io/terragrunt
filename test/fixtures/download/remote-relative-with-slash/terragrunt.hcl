inputs = {
  name = "World"
}

terraform {
  source = "github.com/gruntwork-io/terragrunt.git?ref=c574388b2f2821c10c77f0547570b57e32ef02a0//test/fixtures/download/relative"
}
