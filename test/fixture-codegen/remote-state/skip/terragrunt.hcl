remote_state {
  backend = "local"
  generate = {
    # Intentionally named main.tf so that it conflicts
    path      = "main.tf"
    if_exists = "skip"
  }
  config = {
    path = "foo.tfstate"
  }
}

terraform {
  source = "../../module"
}
