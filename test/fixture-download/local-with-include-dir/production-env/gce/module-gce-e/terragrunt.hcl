inputs = {
  name = "Module GCE E"
}

terraform {
  source = "../../../..//hello-world-no-remote"
}

dependencies {
  paths = ["../../aws/module-aws-d"]
}

