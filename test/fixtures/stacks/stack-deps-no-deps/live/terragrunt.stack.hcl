# Experiment enabled, multiple units, zero autoinclude / dependencies.
# Pins that turning the experiment on is inert when no autoinclude is used.
unit "alpha" {
  source = "../catalog/units/alpha"
  path   = "alpha"
}

unit "beta" {
  source = "../catalog/units/beta"
  path   = "beta"
}
