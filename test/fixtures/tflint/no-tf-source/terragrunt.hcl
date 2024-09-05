terraform {

  before_hook "tflint" {
    commands = ["apply", "plan"]
    execute = ["tflint"]
  }
}

inputs = {
  aws_region = "us-west-2"
  env = "dev"
}
