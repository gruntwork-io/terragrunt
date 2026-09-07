stack "team" {
  expansion {
    for_each = toset(["east", "west"])
  }

  source = "../modules/team"
  path   = "team/${each.key}"
}
