stack "core" {
  source = "${get_repo_root()}/stacks/core"
  path   = "core"
}

unit "cluster" {
  source = "${get_repo_root()}/units/eks"
  path   = "cluster"

  autoinclude {
    dependency "vpc" {
      config_path = stack.core.path

      mock_outputs_allowed_terraform_commands = ["init", "validate", "plan", "apply"]
      mock_outputs = {
        vpc = {
          vpc_id             = "vpc-mock"
          private_subnet_ids = ["subnet-mock-a", "subnet-mock-b"]
        }
      }
    }

    inputs = {
      vpc_id     = try(values.vpc_id, dependency.vpc.outputs.vpc.vpc_id)
      subnet_ids = try(values.subnet_ids, dependency.vpc.outputs.vpc.private_subnet_ids)
    }
  }
}
