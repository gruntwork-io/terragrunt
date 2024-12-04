errors {
  ignore "baz" {
    ignorable_errors = [
      "!.*Error: baz.*" # If STDERR includes "Error: baz", do not ignore it
    ]
    message = "Error handler baz"
  }

}