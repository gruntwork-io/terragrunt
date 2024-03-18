inputs = {
  name = "Module D"
}

terraform {
  source = "../../hello"
}

dependencies {
  paths = ["../module-c"]
}
