locals {
  mock_vars = read_terragrunt_config(find_in_parent_folders("mock.hcl"))
  mock       = local.mock_vars.locals.mock
}

unit "foo" {
  source = "../../units/foo"
  path   = "foo"

  values = {
    mock = local.mock
  }
}
