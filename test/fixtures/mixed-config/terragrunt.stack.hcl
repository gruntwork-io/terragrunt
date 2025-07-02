locals {
  test = "test" 
}

unit "app1" {
  source = "units/app"
  path   = "app1"

  values = {
    project    = local.projec
  }
}