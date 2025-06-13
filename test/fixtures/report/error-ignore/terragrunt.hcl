terraform {
  source = "."
}

errors {
  ignore "ignore_everything" {
    ignorable_errors = [".*"]
  }
}
