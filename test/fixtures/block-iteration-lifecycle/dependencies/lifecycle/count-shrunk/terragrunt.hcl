locals {
  shards = ["web", "edge"]
}

dependency "aurora" {
  expansion {
    count = length(local.shards)
  }

  config_path  = "../aurora-${local.shards[count.index]}"
  skip_outputs = true

  mock_outputs = {
    id = local.shards[count.index]
  }
}

inputs = {
  aurora_keys = sort(keys(dependency.aurora))
  ids         = { for key, instance in dependency.aurora : key => instance.outputs.id }
}
