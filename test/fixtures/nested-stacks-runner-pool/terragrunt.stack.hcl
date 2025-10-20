# Top-level stack that includes nested stacks
# This reproduces the structure from bug #4977

locals {
  stack_root = "${get_terragrunt_dir()}/.terragrunt-stack"

  dependency_path = {
    id        = "${local.stack_root}/id"
    ecr-cache = "${local.stack_root}/ecr-cache"
  }
}

# Top-level units
unit "id" {
  source = "${path_relative_from_include()}/units/id"
  path   = "id"
}

unit "ecr-cache" {
  source = "${path_relative_from_include()}/units/ecr-cache"
  path   = "ecr-cache"

  values = {
    dependency_path = local.dependency_path
  }
}

# Nested stacks
stack "network" {
  source = "${path_relative_from_include()}/stacks/network"
  path   = "network"

  values = {
    dependency_path = local.dependency_path
  }
}

stack "k8s" {
  source = "${path_relative_from_include()}/stacks/k8s"
  path   = "k8s"

  values = {
    # Pass dependencies to the k8s stack units
    dependencies = [
      "${local.stack_root}/network/.terragrunt-stack/vpc-nat",
      "${local.stack_root}/network/.terragrunt-stack/tailscale-router",
    ]

    dependency_path = merge(
      local.dependency_path,
      {
        vpc = "${local.stack_root}/network/.terragrunt-stack/vpc"
      }
    )
  }
}
