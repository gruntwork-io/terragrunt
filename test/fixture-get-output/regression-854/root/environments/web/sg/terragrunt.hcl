include {
  path = "../../../terragrunt.hcl"
}

dependency "network" {
  config_path = "../../network"
}
