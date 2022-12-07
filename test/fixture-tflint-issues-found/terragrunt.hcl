terraform {
  source = "."

  before_hook "tflint" {
    commands = ["apply", "plan"]
    execute = ["tflint"]
  }
}

inputs = {
  instance_class = "db.m3.invalid"
}