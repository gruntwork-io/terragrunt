# Test template with both shell template functions and hooks
terraform {
  source = "git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs?ref=v0.53.8"
}

inputs = {
  test_var = "{{ .TestVariable }}"
  # This uses the shell template function
  current_user = "{{ shell "whoami" }}"
  timestamp = "{{ shell "date +%s" }}"
}
