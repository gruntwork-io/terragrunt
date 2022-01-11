locals {
  json = jsondecode(sops_decrypt_file("${get_terragrunt_dir()}/secrets.json"))
  yaml = yamldecode(sops_decrypt_file("${get_terragrunt_dir()}/secrets.yaml"))
  text = sops_decrypt_file("${get_terragrunt_dir()}/secrets.txt")
  env  = sops_decrypt_file("${get_terragrunt_dir()}/secrets.env")
  ini  = sops_decrypt_file("${get_terragrunt_dir()}/secrets.ini")
}

inputs = {
  json_string_array = local.json["example_array"]
  json_bool_array   = local.json["example_booleans"]
  json_string       = local.json["example_key"]
  json_number       = local.json["example_number"]
  json_hello        = local.json["hello"]
  yaml_string_array = local.yaml["example_array"]
  yaml_bool_array   = local.yaml["example_booleans"]
  yaml_string       = local.yaml["example_key"]
  yaml_number       = local.yaml["example_number"]
  yaml_hello        = local.yaml["hello"]
  text_value        = local.text
  env_value         = local.env
  ini_value         = local.ini
}
