terraform {
  no_dot_terragrunt_cache = true
  source   = "./app"
}

inputs = {
  test_value = "no-cache-with-source-test"
}
