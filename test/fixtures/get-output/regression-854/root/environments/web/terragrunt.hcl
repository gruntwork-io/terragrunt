include {
  path = "../../terragrunt.hcl"
}

dependency "network" {
  config_path = "../network"
}

dependency "sg" {
  config_path = "./sg"
}
