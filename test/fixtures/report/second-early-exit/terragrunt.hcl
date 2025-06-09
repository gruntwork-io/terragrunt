terraform {
  source = "."
}

dependency "second_failure" {
  config_path = "../second-failure"
}
