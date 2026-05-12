# Test template with hooks
terraform {
  source = "git::__MIRROR_URL__//test/fixtures/inputs?ref=v0.53.8"
}

inputs = {
  test_var = "{{ .TestVariable }}"
}
