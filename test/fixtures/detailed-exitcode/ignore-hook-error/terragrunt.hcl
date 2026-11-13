terraform {
  after_hook "fail" {
    commands = ["plan"]
    execute  = ["sh", "-c", "exit 1"]
  }
}

errors {
  ignore "hook_failure" {
    ignorable_errors = [".*"]
    message          = "Ignoring hook failure"
  }
}
