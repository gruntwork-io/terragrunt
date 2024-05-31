
locals {
  secret_vars = yamldecode(sops_decrypt_file("file.enc.yaml"))
}