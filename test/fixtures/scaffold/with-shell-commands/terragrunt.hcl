# Test template with shell template function
terraform {
  source = "git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs?ref=v0.53.8"
}

inputs = {
  test_var = "{{ .TestVariable }}"
  # This should execute a shell command when NoShell = false
  current_date = "{{ shell "date +%Y-%m-%d" }}"
  # This should execute another shell command
  whoami = "{{ shell "whoami" }}"
}
