unit "vpc" {
  source = "${get_repo_root()}/units/vpc"
  path   = "vpc"
}

unit "subnets" {
  source = "${get_repo_root()}/units/subnets"
  path   = "subnets"
}
