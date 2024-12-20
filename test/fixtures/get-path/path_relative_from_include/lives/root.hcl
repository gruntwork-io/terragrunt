locals {
  org_vars = read_terragrunt_config("${get_parent_terragrunt_dir()}/org.hcl")
  env_vars = read_terragrunt_config("${get_parent_terragrunt_dir()}/dev/env.hcl")
  tier_vars = read_terragrunt_config("tier.hcl")

  organization_unit = local.org_vars.locals.organization_unit
  environment       = local.env_vars.locals.environment
  tier              = local.tier_vars.locals.tier
}

generate "provider" {
  path      = "terraform.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<-EOF
    terraform {
      backend "local" {
        path = "${local.environment}-${local.tier}.state"
      }
    }
    EOF
}

inputs = merge(
  local.org_vars.locals,
  local.env_vars.locals,
  local.tier_vars.locals,
  )
