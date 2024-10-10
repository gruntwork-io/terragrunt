provider "null" {}

terraform {
  required_providers {
    null = {
      source  = "registry.terraform.io/hashicorp/null"
      version = "2.1.2"
    }
  }
}


# Create a large string by repeating a smaller string multiple times
resource "null_resource" "large_json" {
  count = 1

  triggers = {
    large_data = join("", [
      for i in range(0, 1024) : "ThisIsAVeryLongStringRepeatedManyTimesToCreateLargeDataBlock_"
    ])
  }
}

resource "null_resource" "large_json_2" {
  count = 1

  triggers = {
    large_data = join("", [
      for i in range(0, 1024) : "ThisIsAVeryLongStringRepeatedManyTimesToCreateLargeDataBlock_1024"
    ])
  }
}


output "large_json_output" {
  value = null_resource.large_json[0].triggers.large_data
}


output "large_json_output_2" {
  value = null_resource.large_json_2[0].triggers.large_data
}
