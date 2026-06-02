terraform {
  source = "."
}

# The unit declares its own same-name dependency with unit-side mock outputs. Under shallow merge the
# autoinclude wins by name and replaces this whole block, so the unit-only key (from_unit) is dropped
# and the conflicting key (common) resolves to the autoinclude's value, not deep-merged.
dependency "x" {
  config_path = "../x"

  mock_outputs = {
    from_unit = "unitval"
    common    = "unit-common"
  }
}

# Backend key references only the autoinclude's surviving outputs:
#   from_autoinclude  -> autoinclude-only key, present after the replacement
#   common            -> conflicting key, resolves to the autoinclude value
# It deliberately does NOT reference from_unit: under shallow merge that key no longer exists, which is
# exactly what proves the unit's block was replaced rather than deep-merged.
remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    path = "${dependency.x.outputs.from_autoinclude}-${dependency.x.outputs.common}.tfstate"
  }
}
