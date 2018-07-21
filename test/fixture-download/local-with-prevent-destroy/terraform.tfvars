name = "World"

terragrunt = {
  terraform {
    source = "../hello-world"
  }

  prevent_destroy = true
}
