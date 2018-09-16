name = "Module GCE E"

terragrunt = {
  terraform {
    source = "../../../..//hello-world-no-remote"
  }

  dependencies {
    paths = ["../../aws/module-aws-d"]
  }
}
