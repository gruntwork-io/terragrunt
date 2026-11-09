outputs {
  mock_strategy {
    allowed_commands = ["plan"]
    merge_with_state = "shallow"
  }

  output "vpc_id" {
    mock_value = "vpc-mock"
  }

  output "nat_id" {
    mock_value = "nat-mock"
  }
}
