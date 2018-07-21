name = "Module B"

terragrunt = {
  terraform {
    source = "../../hello-world"
  }

  dependencies {
    paths = ["../module-a"]
  }

  prevent_destroy = true
}
