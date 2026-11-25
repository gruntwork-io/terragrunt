outputs {
  mock_strategy {
    allowed_commands = ["validate", "plan"]
  }

  output "vpc_id" {
    mock_value = "vpc-mock"
  }
}
