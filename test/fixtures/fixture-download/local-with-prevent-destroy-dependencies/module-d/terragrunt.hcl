inputs = {
  name = "Module D"
}

terraform {
  source = "../../hello-world"
}

dependencies {
  paths = ["../module-c"]
}
