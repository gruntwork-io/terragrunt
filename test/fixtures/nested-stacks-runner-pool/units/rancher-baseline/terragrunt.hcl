terraform {
  source = "."
}

dependency "rancher-bootstrap" {
  config_path = "../rancher-bootstrap"

  mock_outputs = {
    bootstrap_status = "mock-bootstrap-status"
  }
}

inputs = {
  bootstrap_status = dependency.rancher-bootstrap.outputs.bootstrap_status
}
