dependency "aurora" {
  config_path  = "../aurora-web"
  skip_outputs = true

  mock_outputs = {
    id = "aurora-web"
  }
}

inputs = {
  addresses = sort(keys(dependency))
  aurora_id = dependency.aurora.outputs.id
}
