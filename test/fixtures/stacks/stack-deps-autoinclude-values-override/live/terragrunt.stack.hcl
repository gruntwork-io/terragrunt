unit "vpc" {
  source = "${get_repo_root()}/units/vpc"
  path   = "vpc"

  values = {
    name = "test"
  }
}

unit "subnet" {
  source = "${get_repo_root()}/units/subnet"
  path   = "subnet"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
      mock_outputs = {
        vpc_id = "mock-vpc-id"
      }
    }

    inputs = {
      vpc_id = dependency.vpc.outputs.vpc_id
    }
  }

  values = {
    cidr = "10.0.0.0/24"
  }
}
