variables {
  kind   = "tfvar"
  inputs = { from_tfvar = "tfvar" }
}

variables {
  kind   = "json"
  inputs = { from_json = "json" }
}

variables {
  inputs = { from_env = "env" }
}
