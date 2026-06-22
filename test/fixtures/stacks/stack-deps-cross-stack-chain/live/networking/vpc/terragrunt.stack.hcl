# Top-level networking stack holding a single vpc unit.
unit "vpc" {
  source = "${get_repo_root()}/units/aws-vpc"
  path   = "vpc"
}
