variables {
  nullable = true
  inputs   = { env_kept = null }
}

variables {
  kind     = "tfvar"
  nullable = true
  inputs   = { tfvar_kept = null }
}

variables {
  inputs = { env_dropped = null }
}

variables {
  kind   = "tfvar"
  inputs = { tfvar_dropped = null }
}
