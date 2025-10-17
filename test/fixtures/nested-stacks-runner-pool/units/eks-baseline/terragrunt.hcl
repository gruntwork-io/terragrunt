terraform {
  source = "."
}

dependency "eks-cluster" {
  config_path = "../eks-cluster"

  mock_outputs = {
    cluster_id = "mock-cluster-id"
  }
}

inputs = {
  cluster_id = dependency.eks-cluster.outputs.cluster_id
}
