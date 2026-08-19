dependency "vpc" {
  config_path = "../vpc"
}

dependency "aurora" {
  expansion {
    for_each = toset(["web", "api"])
  }

  config_path = "../aurora-${each.key}"
}

dependency "shard" {
  expansion {
    count = 2
  }

  config_path = "../shard-${count.index}"
}

inputs = {
  vpc_id       = dependency.vpc.outputs.id
  web_id       = dependency.aurora["web"].outputs.id
  api_id       = dependency.aurora["api"].outputs.id
  shard_first  = dependency.shard["0"].outputs.id
  shard_second = dependency.shard["1"].outputs.id
}
