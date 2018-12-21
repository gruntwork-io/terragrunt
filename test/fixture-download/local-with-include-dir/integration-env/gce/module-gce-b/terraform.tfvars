name = "Module GCE B"

terragrunt = {
  terraform {
    source = "../../../..//hello-world-no-remote"
  }

  dependencies {
    paths = ["../../aws/module-aws-a"]
  }
}
