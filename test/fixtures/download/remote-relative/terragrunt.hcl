inputs = {
  name = "World"
}

terraform {
  source = "github.com/gruntwork-io/terragrunt.git//test/fixtures/download/relative?ref=c574388b2f2821c10c77f0547570b57e32ef02a0"
}
