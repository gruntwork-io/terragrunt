unit "vpc" {
  enabled = true

  source = "../units/app"
  path   = "vpc"

  values = {
    role = "vpc"
  }
}

unit "legacy" {
  enabled = false

  source = "../units/app"
  path   = "legacy"

  values = {
    role = "legacy"
  }
}

unit "shard" {
  expansion {
    for_each = toset(["web", "api"])
  }

  enabled = false

  source = "../units/app"
  path   = "shard/${each.key}"

  values = {
    role = "shard-${each.key}"
  }
}
