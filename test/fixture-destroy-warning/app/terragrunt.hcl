dependency "vpc" {
  config_path = "../vpc"
  mock_outputs_allowed_terraform_commands = ["validate"]
  mock_outputs = {
    vpc = "mock"
  }
}

dependencies {
  paths = ["../vpc"]
}
