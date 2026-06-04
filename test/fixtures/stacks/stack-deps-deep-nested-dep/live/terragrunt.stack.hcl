# Sandbox references the eks stack, which itself references the core stack: two levels of nested
# .terragrunt-stack directories. A unit autoinclude in the core stack depends on a sibling unit via
# unit.X.path; that config_path must account for BOTH nested .terragrunt-stack segments.
stack "eks" {
  source = "${get_repo_root()}/stacks/eks"
  path   = "eks"
}
