remote_state {
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  backend = "local"
  config = {
    workspace_dir  = "."
    path = "foo.tfstate"
  }
}
