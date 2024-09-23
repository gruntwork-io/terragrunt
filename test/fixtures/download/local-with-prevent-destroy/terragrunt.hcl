inputs = {
  name = "World"
}

terraform {
  source = "../hello-world"
}

prevent_destroy = true
