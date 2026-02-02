# Unit B that includes root.hcl
# The run_cmd in root.hcl should have its output visible

include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}
