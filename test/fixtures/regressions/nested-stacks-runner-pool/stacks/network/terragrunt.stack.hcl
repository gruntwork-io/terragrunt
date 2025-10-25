
unit "vpc" {
  path   = "vpc"
  source = "${get_repo_root()}/_source/units/vpc"
  values = values
}

unit "vpc-nat" {
  path   = "vpc-nat"
  source = "${get_repo_root()}/_source/units/vpc-nat"
  values = values
}

unit "vpc-endpoints" {
  path   = "vpc-endpoints"
  source = "${get_repo_root()}/_source/units/vpc-endpoints"
  values = values
}

unit "tailscale-router" {
  path   = "tailscale-router"
  source = "${get_repo_root()}/_source/units/tailscale-router"
  values = values
}


