locals {
  secret = try(jsondecode(sops_decrypt_file("${get_terragrunt_dir()}/secret.enc.json")), {})
}

inputs = {
  secret_value = lookup(local.secret, "value", "DECRYPTION_FAILED")
  unit_name    = "unit-20"
}
