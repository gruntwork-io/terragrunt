locals {
    secrets = yamldecode(sops_decrypt_file("sops.yaml"))
}
