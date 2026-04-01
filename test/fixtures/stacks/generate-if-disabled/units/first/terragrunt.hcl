generate "alpha" {
  path        = "alpha.tf"
  if_exists   = "skip"
  if_disabled = "remove"
  contents    = ""
  disable     = values.provider != "alpha"
}

generate "beta" {
  path        = "beta.tf"
  if_exists   = "skip"
  if_disabled = "remove"
  contents    = ""
  disable     = values.provider != "beta"
}
