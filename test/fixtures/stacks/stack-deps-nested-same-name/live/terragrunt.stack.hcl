locals {
  env = "test"
}

stack "foo" {
  source = "../catalog/stacks/foo"
  path   = "foo"
}

unit "bar" {
  source = "../catalog/units/bar"
  path   = "bar"

  autoinclude {
    # stack.foo.path     -> the stack root  (e.g. /abs/.terragrunt-stack/foo)
    # stack.foo.foo.path -> the unit "foo" inside stack "foo"
    #                       (e.g. /abs/.terragrunt-stack/foo/.terragrunt-stack/foo)
    dependency "foo_unit" {
      config_path = stack.foo.foo.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        val = "fake-val"
      }
    }

    inputs = {
      val = dependency.foo_unit.outputs.val
    }
  }
}
