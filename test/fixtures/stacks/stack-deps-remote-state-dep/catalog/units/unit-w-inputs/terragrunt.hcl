terraform {
  source = "."
}

remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    path = "${dependency.unit_w_outputs.outputs.val}.tfstate"
  }
}
