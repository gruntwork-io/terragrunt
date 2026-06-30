dependency "dep" {
  config_path = "${get_terragrunt_dir()}/../unreachable-dep"
}

locals {
  common = "shared"
}
