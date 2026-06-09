# Test template with both shell template functions and hooks
terraform {
  source = "git::__MIRROR_URL__//test/fixtures/inputs?ref=v0.53.8"
}

inputs = {
  test_var = "{{ .TestVariable }}"
  shell_result_1 = "{{ shell "echo" "-n" "SHELL_OUTPUT_1" }}"
  shell_result_2 = "{{ shell "echo" "-n" "SHELL_OUTPUT_2" }}"
}
