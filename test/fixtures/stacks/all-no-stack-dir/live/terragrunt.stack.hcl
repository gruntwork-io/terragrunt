unit "foo" {
  source                  = "../unit"
  path                    = "foo"
  no_dot_terragrunt_stack = true
}

unit "bar" {
  source                  = "../unit"
  path                    = "bar"
  no_dot_terragrunt_stack = true
}

