inputs = {
  name = "Module GCE B"
}

terraform {
  source = "../../../..//hello-world-no-remote"
}

dependencies {
  paths = ["../../aws/module-aws-a"]
}
