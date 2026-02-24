# Minimal terragrunt config without terraform.source block
# This tests that .terragrunt-cache is still created when no source is specified

inputs = {
  test_value = "no-source-test"
}
