outputs {
  mock_strategy {
    allowed_commands = ["validate", "plan"]
  }

  output "vpc_id" {
    type       = string
    mock_value = "vpc-mock"
  }

  output "port" {
    type       = string
    mock_value = 8080
  }

  output "secret" {
    mock_value = "secret-mock"

    mock_strategy {
      allowed_commands = ["validate"]
    }
  }
}
