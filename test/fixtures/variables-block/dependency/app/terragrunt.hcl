dependency "net" {
  config_path = "../net"

  mock_outputs = {
    network = { id = "mock", cidr = "mock" }
  }
}

variables {
  kind     = "tfvar"
  validate = true

  inputs = {
    network_id = dependency.net.outputs.network.id
    network    = dependency.net.outputs.network
  }
}
