unit "vpc" {
  source = "${get_repo_root()}/units/vpc"
  path   = "vpc"
}

stack "subnets" {
  source = "${get_repo_root()}/stacks/subnets"
  path   = "subnets"
}
