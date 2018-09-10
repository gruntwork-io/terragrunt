name = "Module GCE E"

terragrunt = {
  terraform {
    source = "../../../../hello-world"
  }

  dependencies {
    paths = ["../../aws/module-aws-d"]
  }
}
