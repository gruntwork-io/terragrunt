unit "aurora" {
  expansion {
    for_each = toset(["api"])
  }

  source = "../modules/app"
  path   = "aurora/${each.key}"

  values = {
    role = each.key
  }
}
