terraform {
  include_in_copy = [".terraform-version"]


  before_hook "tflint" {
    commands = ["apply", "plan", "verify"]
    execute  = ["tflint"]
  }


}
