dependency "aurora" {
  expansion {
    for_each = toset(["api", "web"])
  }

  config_path  = "../aurora-${each.key}"
  skip_outputs = true

  mock_outputs = {
    id = "aurora-${each.key}"
  }
}

inputs = {
  addresses   = sort(keys(dependency))
  aurora_keys = sort(keys(dependency.aurora))
  ids         = { for key, instance in dependency.aurora : key => instance.outputs.id }
}
