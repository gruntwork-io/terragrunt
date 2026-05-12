# Mock dependency configuration
terraform {
  source = "git::__MIRROR_SSH_URL__//test/fixtures/download/hello-world"
}

inputs = {
  some_value = "test-value"
}
