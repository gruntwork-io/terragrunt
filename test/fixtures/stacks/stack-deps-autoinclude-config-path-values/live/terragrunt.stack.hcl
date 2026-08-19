unit "vpc" {
  source = "../catalog/vpc"
  path   = "vpc"
}

unit "app" {
  source = "../catalog/app"
  path   = "app"

  # values intentionally omits vpc_path — the autoinclude provides the dependency override.
  values = {
    region = "us-east-1"
  }

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path

      mock_outputs_allowed_terraform_commands = ["init", "plan", "validate"]
      mock_outputs = {
        vpc_id = "from-autoinclude"
      }
    }
  }
}
