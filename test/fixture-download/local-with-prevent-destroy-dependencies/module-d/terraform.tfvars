name = "Module D"

terragrunt = {
  terraform {
    source = "../../hello-world"
  }

  dependencies {
    paths = ["../module-c"]
  }
}
