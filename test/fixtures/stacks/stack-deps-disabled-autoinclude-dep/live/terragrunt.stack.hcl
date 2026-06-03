# A dependency declared in an autoinclude with enabled = false must not create a run-DAG edge. The
# dependency points at a path that does not exist, so if the disabled block were followed the run
# would fail with a missing terragrunt.hcl error. enabled = false must drop it from the run graph.
unit "svc" {
  source = "../catalog/units/svc"
  path   = "svc"

  autoinclude {
    dependency "ghost" {
      config_path = "../nonexistent-in-tree"
      enabled     = false
    }
  }
}
