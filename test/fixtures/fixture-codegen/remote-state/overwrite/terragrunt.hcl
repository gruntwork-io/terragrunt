remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    path = "foo.tfstate"
  }
}

terraform {
  source = "../../module"
}
