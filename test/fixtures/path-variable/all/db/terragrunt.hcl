locals {
  module = file("${unit.path}/main.tf")
}

remote_state {
  backend = "local"

  config = {
    path = "${unit.path}/terraform.tfstate"
  }
}

inputs = {
  unit = unit.path
}
