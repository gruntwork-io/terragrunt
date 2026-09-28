dependency "vpc" {
  config_path  = "../vpc"
  skip_outputs = true
}

dependency "aurora" {
  expansion {
    for_each = toset(["web", "api"])
  }

  config_path  = "../aurora-${each.key}"
  skip_outputs = true
}

dependency "shard" {
  expansion {
    count = 2
  }

  config_path  = "../shard-${count.index}"
  skip_outputs = true
}

inputs = {
  name = "app"
}
