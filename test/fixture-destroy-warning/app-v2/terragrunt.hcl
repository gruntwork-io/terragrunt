dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    vpc = "mock"
  }
}

dependencies {
  paths = ["../vpc"]
}
