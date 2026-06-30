# The dependency reference sits in an untaken conditional branch: the source evaluates to "./module", but a static
# scan of the expression still sees the dependency namespace. Discovery must not let that abort dependency detection.
terraform {
  source = true ? "./module" : dependency.upstream.outputs.source
}

dependency "upstream" {
  config_path  = "../upstream"
  mock_outputs = { source = "./module" }
}
