include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "producer" {
  config_path = "../producer"

  mock_outputs = {
    producer_value = "mock-gcs-value"
  }
}

inputs = {
  producer_value = dependency.producer.outputs.producer_value
}
