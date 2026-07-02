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

# Backend key is built so shallow and deep merge produce DIFFERENT, both-valid results:
#   from_unit (via try) -> unit-only key. Under shallow merge the autoinclude block REPLACES the unit's,
#                          so from_unit no longer exists and try() falls back to "absent". Under a deep
#                          merge from_unit would survive as "unitval", so the asserted value diverges.
#   common              -> conflicting key, resolves to the autoinclude (winning side) value.
remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    path = "${try(dependency.x.outputs.from_unit, "absent")}-${dependency.x.outputs.common}.tfstate"
  }
}
