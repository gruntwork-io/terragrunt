unit "data" {
  source = "${get_repo_root()}/units/data"
  path   = "data"
}

unit "vpc" {
  source = "${get_repo_root()}/units/vpc"
  path   = "vpc"

  autoinclude {
    dependency "data" {
      config_path = unit.data.path

      mock_outputs = {
        availability_zones = ["region-a", "region-b", "region-c"]
      }
    }

    inputs = {
      availability_zones = try(values.availability_zones, dependency.data.outputs.availability_zones)
    }
  }

  values = {
    vpc_cidr = "172.29.0.0/16"
  }
}
