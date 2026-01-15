terraform {
  source = "."
}

locals {
  source_config = read_terragrunt_config("../../sources/source.hcl")
}

