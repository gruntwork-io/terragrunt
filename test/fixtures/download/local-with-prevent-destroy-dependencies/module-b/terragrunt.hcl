inputs = {
  name = "Module B"
}

terraform {
  source = "../../hello-world"
}

dependencies {
  paths = ["../module-a"]
}

prevent_destroy = true
