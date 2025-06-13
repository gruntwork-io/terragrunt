terraform {
  source = "."
}

dependency "chain_b" {
  config_path = "../chain-b"
}
