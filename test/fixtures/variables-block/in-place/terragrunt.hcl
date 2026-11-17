terraform {
  no_dot_terragrunt_cache = true
}

variables {
  kind   = "tfvar"
  inputs = { from_tfvar = "tfvar" }
}

variables {
  kind   = "json"
  inputs = { from_json = "json" }
}
