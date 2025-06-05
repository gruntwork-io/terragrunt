terraform {
  source = "."
}

dependency "first_failure" {
  config_path = "../first-failure"
}
