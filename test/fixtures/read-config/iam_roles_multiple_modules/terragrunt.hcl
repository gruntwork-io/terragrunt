
dependency "component1" {
  config_path = "${get_parent_terragrunt_dir()}/component1"
}

dependency "component2" {
  config_path = "${get_parent_terragrunt_dir()}/component2"
}
