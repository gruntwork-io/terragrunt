terraform {
  source = "."

  before_hook "tflint" {
    commands = ["apply", "plan"]
    execute = ["tflint" , "--external-tflint"]
  }

  extra_arguments "var-files" {
    commands = ["apply", "plan"]
    required_var_files = ["extra.tfvars"]
  }
}

