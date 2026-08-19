dependency "aurora" {
  expansion {
    for_each = toset(["web", "api"])
  }

  config_path  = "../aurora-${each.key}"
  skip_outputs = true

  mock_outputs = {
    id = "mock-${each.key}"
  }
}

inputs = {
  web_id = dependency.aurora["web"].outputs.id
  api_id = dependency.aurora["api"].outputs.id
}
