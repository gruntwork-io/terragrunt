terraform {
  source = "."

  before_hook "tflint" {
    commands = ["apply", "plan"]
    execute = ["tflint"]
  }
}

inputs = {
  aws_region = "eu-central-1"
  env = "dev"
}