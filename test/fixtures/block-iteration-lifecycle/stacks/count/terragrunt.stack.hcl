stack "team" {
  expansion {
    count = 2
  }

  source = "../modules/team"
  path   = "team/${count.index}"
}
