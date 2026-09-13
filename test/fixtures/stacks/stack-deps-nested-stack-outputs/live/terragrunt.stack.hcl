# A unit depends on the whole network stack and reads an output of a unit that lives in the
# network stack's nested subnets stack.

stack "network" {
  source = "../stacks/network"
  path   = "network"
}

unit "app" {
  source = "../units/app"
  path   = "app"

  autoinclude {
    dependency "network" {
      config_path = stack.network.path
    }

    inputs = {
      vpc_id    = dependency.network.outputs.vpc.vpc_id
      subnet_id = dependency.network.outputs.subnets.subnet.subnet_id
    }
  }
}
