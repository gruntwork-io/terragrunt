terraform {
  source = "."
}

dependency "chain_a" {
  config_path = "../chain-a"
}
