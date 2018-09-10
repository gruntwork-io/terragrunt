name = "Module GCE B"

terragrunt = {
  terraform {
    source = "../../../../hello-world"
  }

  dependencies {
    paths = ["../../aws/module-aws-a"]
  }
}
