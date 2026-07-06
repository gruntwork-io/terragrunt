# The dependency reference in `source` is rejected: the module source must be resolvable before dependencies are
# evaluated, and a static scan flags the reference even though this branch is untaken. Discovery must degrade
# gracefully (list the unit without aborting) rather than crash.
terraform {
  source = true ? "./module" : dependency.upstream.outputs.source
}

dependency "upstream" {
  config_path  = "../upstream"
  mock_outputs = { source = "./module" }
}
