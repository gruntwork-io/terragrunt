terraform {
  source = "."
}

# Dependencies from which we may or may not consume outputs
dependencies {
  paths = try(values.dependencies, [])
}

# CRITICAL: This dependency uses values.dependency_path which points to the GENERATED unit path
# This creates a cycle because:
# - eks-baseline (in stacks/k8s) depends on eks-cluster via values.dependency_path
# - values.dependency_path.eks-cluster = "./.terragrunt-stack/k8s/.terragrunt-stack/eks-cluster"
# - But eks-baseline itself gets generated to "./.terragrunt-stack/k8s/.terragrunt-stack/eks-baseline"
# This inter-stack dependency via generated paths triggers the cycle detection bug
dependency "eks-cluster" {
  config_path = values.dependency_path.eks-cluster

  mock_outputs = {
    cluster_id = "mock-cluster-id"
  }

  mock_outputs_merge_strategy_with_state  = "shallow"
  mock_outputs_allowed_terraform_commands = ["init", "validate", "destroy", "plan"]
}

inputs = {
  cluster_id = dependency.eks-cluster.outputs.cluster_id
}
