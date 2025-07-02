locals {
  test = "test"
}

unit "app1" {
  source = "units/app"
  path   = "app1"

  values = {
    not_existing_variable    = local.not_existing_variable
  }
}