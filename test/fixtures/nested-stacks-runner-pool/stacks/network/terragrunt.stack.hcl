# Network nested stack with multiple units
# Units have dependencies to create a proper DAG

unit "vpc" {
  source = "${path_relative_from_include()}/../../units/vpc"
  path   = "vpc"
}

unit "tailscale-router" {
  source = "${path_relative_from_include()}/../../units/tailscale-router"
  path   = "tailscale-router"
}

unit "vpc-endpoints" {
  source = "${path_relative_from_include()}/../../units/vpc-endpoints"
  path   = "vpc-endpoints"
}

unit "vpc-nat" {
  source = "${path_relative_from_include()}/../../units/vpc-nat"
  path   = "vpc-nat"
}
