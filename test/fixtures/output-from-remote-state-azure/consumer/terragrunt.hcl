include "root" {
  path = find_in_parent_folders("common.hcl")
}

dependency "producer" {
  config_path = "../producer"

  mock_outputs = {
    producer_value = "mock-azure-value"
  }
}

inputs = {
  producer_value = dependency.producer.outputs.producer_value
}
