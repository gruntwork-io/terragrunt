outputs {
  mock_strategy {
    allowed_commands = ["validate", "plan"]
  }

  output "vpc_id" {
    mock_value = "vpc-mock"
  }

  output "subnet_ids" {
    mock_value = ["subnet-mock-1", "subnet-mock-2"]
  }
}
