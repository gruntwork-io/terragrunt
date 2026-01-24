remote_state {
  backend = "http"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    address        = "__HTTP_SERVER_URL__/state/${path_relative_to_include()}"
    lock_address   = "__HTTP_SERVER_URL__/state/${path_relative_to_include()}"
    unlock_address = "__HTTP_SERVER_URL__/state/${path_relative_to_include()}"
    username       = "admin"
    password       = "secret"
  }
  encryption = {
    key_provider = "pbkdf2"
    passphrase   = "testpassphrase123"
  }
}
