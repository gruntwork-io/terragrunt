remote_state {
  backend = "local"
  config = {
    path = "${path_relative_to_include()}/terraform.tfstate"
  }
}

download_dir = "${get_terragrunt_dir()}/.download"
