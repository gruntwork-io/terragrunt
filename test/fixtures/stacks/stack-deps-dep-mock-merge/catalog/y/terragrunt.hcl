terraform {
  source = "."
}

# The unit's own dependency: wrong path (autoinclude overrides) plus unit-side mock outputs.
dependency "x" {
  config_path = "./this-path-does-not-exist"

  mock_outputs = {
    from_unit = "unitval"
    common    = "unit-common"
  }
}

# Backend key interleaves three mock outputs so the merged values are observable:
#   from_unit         -> unit-only key, must survive the merge
#   from_autoinclude  -> autoinclude-only key, must be present
#   common            -> conflicting key, autoinclude (source) must win
remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    path = "${dependency.x.outputs.from_unit}-${dependency.x.outputs.from_autoinclude}-${dependency.x.outputs.common}.tfstate"
  }
}
