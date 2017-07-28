terragrunt = {
  terraform {
    source = "temp"
  }
  dependencies {
    paths = ["../module-a"]
  }
}
