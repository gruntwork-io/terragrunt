# A top-level stack referencing a nested "core" stack whose unit autoinclude depends on a sibling unit
# via unit.X.path. The generated dependency config_path must account for the nested .terragrunt-stack
# directory, not point one level too high.
stack "core" {
  source = "${get_repo_root()}/stacks/core"
  path   = "core"
}
