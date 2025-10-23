
locals {
  stack_root = "${get_terragrunt_dir()}/.terragrunt-stack"

  dependency_path = {
    id        = "${local.stack_root}/id"
    ecr-cache = "${local.stack_root}/ecr-cache"
  }
}

unit "id" {
  path   = "id"
  source = "${get_repo_root()}/_source/units/id"

  values = {
    prefix = "aio"
  }
}

unit "ecr-cache" {
  path   = "ecr-cache"
  source = "${get_repo_root()}/_source/units/ecr-cache"

  values = {
    dependency_path = local.dependency_path
  }
}

stack "network" {
  path   = "network"
  source = "../stacks/network"

  values = {
    dependency_path = local.dependency_path
  }
}

stack "k8s" {
  path   = "k8s"
  source = "../stacks/k8s"

  values = {
    dependencies = [
      "${local.stack_root}/network/.terragrunt-stack/vpc-nat",
      "${local.stack_root}/network/.terragrunt-stack/tailscale-router",
    ]

    dependency_path = merge(
      local.dependency_path,
      {
        vpc = "${local.stack_root}/network/.terragrunt-stack/vpc",
        eks-cluster = "${local.stack_root}/k8s/.terragrunt-stack/eks-cluster",
        rancher-baseline = "${local.stack_root}/k8s/.terragrunt-stack/rancher-baseline",
      }
    )
  }
}


