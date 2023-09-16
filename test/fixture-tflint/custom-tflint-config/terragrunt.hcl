terraform {
  before_hook "tflint" {
    commands = ["plan"]
    execute  = ["tflint", "--terragrunt-external-tflint", "--config", "custom.tflint.hcl"]
  }
}

inputs = {
  bucket_name = "my-prefix-qwe"
}
