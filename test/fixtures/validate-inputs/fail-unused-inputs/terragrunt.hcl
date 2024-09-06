inputs = {
  input = "hello world"
  unused_input = "I am unused"
}

# the existence of a `source` directive manifests the bug in https://github.com/gruntwork-io/terragrunt/issues/1793
terraform {
  source = "github.com/gruntwork-io/terragrunt.git//test/fixtures/download/hello-world?ref=5a58053a6a08bac1c7b184e21f536a83cd48a3fa"
}