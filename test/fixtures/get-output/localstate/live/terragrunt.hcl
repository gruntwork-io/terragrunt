remote_state {
  backend = "local"
  config = {
    path = "${get_terragrunt_dir()}/${path_relative_to_include()}/terraform.tfstate"
  }
}
