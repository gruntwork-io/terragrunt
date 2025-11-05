include "root" {
  path = find_in_parent_folders("root.hcl")
  expose = true
}

terraform {
  source = "${get_terragrunt_dir()}/."
}

dependency "service-test1" {
  config_path = "../services/test1"
  mock_outputs_allowed_terraform_commands = ["validate", "plan"]
  mock_outputs = {
    service_desired_count_initial_value = null
    service_name = "FAKE-SERVICE-1"
    docker_images = [
        {
            repository = "123456789012.dkr.ecr.us-east-1.amazonaws.com/test1-FAKE"
        }
    ]
  }
  mock_outputs_merge_strategy_with_state = "shallow"
}

locals {
  repository_url = "123456789012.dkr.ecr.us-east-1.amazonaws.com/api-FAKE"
  name = "services-info"
}

inputs = {
  desired_count = {
    test1 = dependency.service-test1.outputs.service_desired_count_initial_value
  }
  use_api_image = {
    test1 = dependency.service-test1.outputs.docker_images[0].repository == local.repository_url
  }
}
