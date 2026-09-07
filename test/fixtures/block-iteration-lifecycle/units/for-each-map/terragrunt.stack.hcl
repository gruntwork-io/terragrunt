unit "aurora" {
  expansion {
    for_each = {
      api = "backend"
      web = "frontend"
    }
  }

  source = "../modules/app"
  path   = "aurora/${each.key}"

  values = {
    role = each.value
  }
}
