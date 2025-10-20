terraform {
  source = "."
}

# Dependencies from which we may or may not consume outputs
dependencies {
  paths = try(values.dependencies, [])
}

dependency "eks-cluster" {
  config_path = try(values.dependency_path.eks-cluster, "../eks-cluster")

  mock_outputs = {
    cluster_id = "mock-cluster-id"
  }

  mock_outputs_merge_strategy_with_state  = "shallow"
  mock_outputs_allowed_terraform_commands = ["init", "validate", "destroy"]
}

inputs = {
  cluster_id = dependency.eks-cluster.outputs.cluster_id
}
