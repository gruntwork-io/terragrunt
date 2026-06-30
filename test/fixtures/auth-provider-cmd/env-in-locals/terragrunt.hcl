locals {
  secret = get_env("AUTH_PROVIDER_SECRET")
}

inputs = {
  secret = local.secret
}
