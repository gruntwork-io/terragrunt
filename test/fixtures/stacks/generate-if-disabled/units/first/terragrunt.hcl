remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    path = "../../first-terraform.tfstate"
  }
}

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
