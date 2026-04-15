# Docs example: Dependencies on entire stacks
# stack.infra.path resolves to the stack's generated directory

stack "infra" {
  source = "../catalog/stacks/infra"
  path   = "infra"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  autoinclude {
    dependency "infra" {
      config_path = stack.infra.path
    }

    inputs = {
      vpc_id = dependency.infra.outputs.vpc.vpc_id
    }
  }
}
