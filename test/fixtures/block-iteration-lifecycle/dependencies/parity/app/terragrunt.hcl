dependency "vpc" {
  enabled     = false
  config_path = "../vpc"

  mock_outputs = {
    id = "vpc-id"
  }
}

dependency "aurora" {
  expansion {
    for_each = toset(["api", "web"])
  }

  config_path  = "../aurora-${each.key}"
  skip_outputs = true

  mock_outputs = {
    id = "aurora-${each.key}-id"
  }
}

inputs = {
  addresses   = sort(keys(dependency))
  aurora_keys = sort(keys(dependency.aurora))
  vpc_id      = dependency.vpc.outputs.id
  web_id      = dependency.aurora["web"].outputs.id
}
