terraform {
  before_hook "tflint" {
    commands = ["plan"]
    execute  = ["tflint", "--terragrunt-external-tflint"]
  }
}

inputs = {
  bucket_name = "my-prefix-qwe"
}
