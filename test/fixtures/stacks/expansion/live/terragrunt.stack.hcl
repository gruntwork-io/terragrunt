unit "aurora" {
  expansion {
    for_each = toset(["web", "api"])
  }

  source = "../units/app"
  path   = "aurora/${each.key}"

  values = {
    role = each.key
  }
}

unit "shard" {
  expansion {
    count = 2
  }

  source = "../units/app"
  path   = "shard/${count.index}"

  values = {
    role = "shard-${count.index}"
  }
}

unit "vpc" {
  source = "../units/app"
  path   = "vpc"

  values = {
    role = "vpc"
  }
}

stack "team" {
  expansion {
    count = 2
  }

  source = "../stacks/team"
  path   = "team/${count.index}"
}
