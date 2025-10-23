# Test template with shell template function
terraform {
  source = "git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs?ref=v0.53.8"
}

inputs = {
  test_var = "{{ .TestVariable }}"
  shell_output_1 = "{{ shell "echo" "-n" "SHELL_EXECUTED_VALUE_1" }}"
  shell_output_2 = "{{ shell "echo" "-n" "SHELL_EXECUTED_VALUE_2" }}"
}
