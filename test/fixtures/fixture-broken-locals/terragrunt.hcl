locals {
  file = yamldecode(sops_decrypt_file("not-existing-file-that-will-fail-locals-evaluating.yaml"))
}
