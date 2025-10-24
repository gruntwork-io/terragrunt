inputs = {
  name = "World"
}

terraform {
  source = "github.com/gruntwork-io/terragrunt.git?ref=4de37b32367af50a3d5612e42a0965d8e477cbec//test/fixtures/download/relative"
}
