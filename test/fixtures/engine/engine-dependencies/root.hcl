remote_state {
  backend = "local"
  generate = {
    path = "backend.gen.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    path = "${get_terragrunt_dir()}/terraform.tfstate"
  }
}
