include {
  path = find_in_parent_folders()
}

terraform {
  source = "${get_terragrunt_dir()}/../../modules/k8s"
}

dependency "eks" {
  config_path  = "${get_terragrunt_dir()}/../../clusters/eks"
  skip_outputs = true
  mock_outputs = {
    random_string = "foo"
  }
}

inputs = {
  cluster = dependency.eks.outputs.random_string
}
