include "root" {
  path = find_in_parent_folders("root.hcl")
}

inputs = { from_attr = "attr" }

variables {
  kind     = "json"
  nullable = true
  validate = true
  inputs   = { shared = "child", nulled = null, template = "$${literal}" }
}

variables {
  inputs = { from_env = "env", dropped = null }
}
