locals {
  region = "us-west-1"
}

dependency "dep" {
  config_path = "${get_terragrunt_dir()}/../dep"
}
