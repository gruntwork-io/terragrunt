# Chain dependent -> intermediate -> upstream, where upstream is a nested stack (a stack directory).
# Resolving dependent walks the chain transitively, so the upstream stack directory must be handled
# instead of parsed as a unit.

stack "upstream" {
  source = "../stacks/upstream"
  path   = "upstream"
}

unit "intermediate" {
  source = "../units/noop"
  path   = "intermediate"

  autoinclude {
    dependency "upstream" {
      config_path = stack.upstream.path
    }

    inputs = {
      value = dependency.upstream.outputs.leaf.value
    }
  }
}

unit "dependent" {
  source = "../units/noop"
  path   = "dependent"

  autoinclude {
    dependency "intermediate" {
      config_path = unit.intermediate.path
    }

    inputs = {
      value = dependency.intermediate.outputs.value
    }
  }
}
