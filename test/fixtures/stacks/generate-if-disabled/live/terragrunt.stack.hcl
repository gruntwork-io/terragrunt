locals {
  provider = "alpha"
}

unit "first" {
  source = "../units/first"
  path   = "first"
  values = local
}

unit "second" {
  source = "../units/second"
  path   = "second"
  values = local
}
