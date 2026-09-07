dependency "aurora" {
  enabled     = false
  config_path = "../aurora-web"

  mock_outputs = {
    id = "aurora-web-id"
  }
}

inputs = {
  addresses = sort(keys(dependency))
  aurora_id = dependency.aurora.outputs.id
}
