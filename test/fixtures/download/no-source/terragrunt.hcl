# This terragrunt.hcl intentionally has no terraform.source block.
# Terragrunt should still create .terragrunt-cache and run from there.

inputs = {
  name = "no-source-test"
}
