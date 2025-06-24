terraform {
  source = "${get_terragrunt_dir()}/tf"
}

inputs = {
  test_input = "native-provider-cache-test"
}
