# Top-level platform stack with a chain of two units.
# eks autoincludes a dependency on the sibling networking stack directory via find_in_parent_folders.
# karpenter autoincludes a dependency on eks via unit.eks.path, forming a transitive chain.

unit "eks" {
  source = "${get_repo_root()}/units/aws-eks"
  path   = "eks"

  autoinclude {
    dependency "networking" {
      config_path = find_in_parent_folders("networking/vpc")

      mock_outputs_allowed_terraform_commands = ["init", "validate", "plan", "apply"]
      mock_outputs = {
        vpc = {
          vpc_id = "mock-vpc"
        }
      }
    }

    inputs = {
      vpc_id = dependency.networking.outputs.vpc.vpc_id
    }
  }
}

unit "karpenter" {
  source = "${get_repo_root()}/units/aws-eks-karpenter"
  path   = "karpenter"

  autoinclude {
    dependency "eks" {
      config_path = unit.eks.path

      mock_outputs_allowed_terraform_commands = ["init", "validate", "plan", "apply"]
      mock_outputs = {
        cluster_id = "mock-cluster"
      }
    }

    inputs = {
      cluster_id = dependency.eks.outputs.cluster_id
    }
  }
}
