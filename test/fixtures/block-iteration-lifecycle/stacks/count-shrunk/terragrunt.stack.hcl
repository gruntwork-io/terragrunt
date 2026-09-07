stack "team" {
  expansion {
    count = 1
  }

  source = "../modules/team"
  path   = "team/${count.index}"
}
