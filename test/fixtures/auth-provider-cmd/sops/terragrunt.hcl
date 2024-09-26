locals {
  data = jsondecode(jsondecode(sops_decrypt_file("secrets.json")).data)
}

inputs = {
  hello = local.data.hello
}

