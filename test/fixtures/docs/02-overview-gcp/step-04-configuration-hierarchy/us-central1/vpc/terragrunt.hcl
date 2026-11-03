include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "tfr:///terraform-google-modules/network/google?version=18.3.1"
}

inputs = {
  network_name = "__FILL_IN_NAME_PREFIX__-vpc-us-central1"

  subnets = [
    {
      subnet_name   = "__FILL_IN_NAME_PREFIX__-private"
      subnet_ip     = "10.0.1.0/24"
      subnet_region = "us-central1"
    },
    {
      subnet_name   = "__FILL_IN_NAME_PREFIX__-public"
      subnet_ip     = "10.0.101.0/24"
      subnet_region = "us-central1"
    },
  ]
}
