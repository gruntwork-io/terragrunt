locals {
  secret_vars = yamldecode(sops_decrypt_file("${get_terragrunt_dir()}/missing.yaml"))
}
