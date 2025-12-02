# This stack references a template module that uses stack variables
# The module/ directory should be excluded to avoid parsing errors

unit "staging" {
  source = "./module"
  path   = "staging"
  values = {
    env = "staging"
  }
}

unit "production" {
  source = "./module"
  path   = "production"
  values = {
    env = "production"
  }
}
