include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "tfr:///terraform-google-modules/cloud-router/google?version=9.1.0"
}

dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  name    = "__FILL_IN_NAME_PREFIX__-router"
  region  = "us-central1"
  network = dependency.vpc.outputs.network_name
}
