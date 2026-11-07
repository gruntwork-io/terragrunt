locals {
  config = yamldecode(file("../config.yml"))
}

inputs = {
  message = local.config.message
}
