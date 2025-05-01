terraform {
  before_hook "tflint" {
    commands = ["plan"]
    execute  = ["tflint", "--external-tflint"]
  }
}

inputs = {
  bucket_name = "my-prefix-qwe"
}
