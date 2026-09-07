dependency "aurora" {
  expansion {
    for_each = {
      api = "backend"
      web = "frontend"
    }
  }

  config_path  = "../aurora-${each.key}"
  skip_outputs = true

  mock_outputs = {
    id = each.value
  }
}

inputs = {
  aurora_keys = sort(keys(dependency.aurora))
  ids         = { for key, instance in dependency.aurora : key => instance.outputs.id }
}
