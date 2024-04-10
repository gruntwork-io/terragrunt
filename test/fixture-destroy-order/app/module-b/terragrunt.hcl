inputs = {
  name = "Module B"
}

terraform {
  source = "../../hello"
}

dependencies {
  paths = ["../module-a"]
}
