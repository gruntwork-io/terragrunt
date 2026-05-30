inputs = {
  name = "World"
}

terraform {
  # Exercises the commit-SHA path in the CAS getter. __MIRROR_SHA__ is
  # substituted to the local git mirror's HEAD commit hash at render time.
  source = "git::__MIRROR_URL__//test/fixtures/download/hello-world-no-remote?ref=__MIRROR_SHA__"
}
