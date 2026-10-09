# Explicit backend block required when remote_state has no generate block.
# Terragrunt validates that a backend block exists; the actual config is
# injected via -backend-config= args from GetTFInitArgs().
terraform {
  backend "s3" {}
}

output "hello" {
  value = "world"
}
