remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    path = "${dependency.sub.outputs.id}-${dependency.rgs.outputs.name}.tfstate"
  }
}
