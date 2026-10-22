include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

locals {
  region = include.root.locals.region
}

terraform {
  source = "tfr:///terraform-google-modules/network/google?version=18.3.1"
}

inputs = {
  network_name = "__FILL_IN_NAME_PREFIX__-vpc-${local.region}"

  subnets = [
    {
      subnet_name   = "__FILL_IN_NAME_PREFIX__-private"
      subnet_ip     = "10.0.1.0/24"
      subnet_region = local.region
    },
    {
      subnet_name   = "__FILL_IN_NAME_PREFIX__-public"
      subnet_ip     = "10.0.101.0/24"
      subnet_region = local.region
    },
  ]
}
