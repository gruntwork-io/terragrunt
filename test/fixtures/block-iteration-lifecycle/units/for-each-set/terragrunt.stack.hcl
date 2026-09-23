unit "aurora" {
  expansion {
    for_each = toset(["api", "web"])
  }

  source = "../modules/app"
  path   = "aurora/${each.key}"

  values = {
    role = each.key
  }
}
