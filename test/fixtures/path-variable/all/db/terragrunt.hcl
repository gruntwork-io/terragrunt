locals {
  module = file("${path.unit}/main.tf")
}

remote_state {
  backend = "local"

  config = {
    path = "${path.unit}/terraform.tfstate"
  }
}

inputs = {
  unit = path.unit
}
