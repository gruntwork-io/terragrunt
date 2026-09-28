unit "vpc" {
  enabled = false

  source = "../units/app"
  path   = "vpc"

  values = {
    role = "vpc"
  }
}

unit "app" {
  enabled = false

  source = "../units/app"
  path   = "app"

  values = {
    role = "app"
  }
}
