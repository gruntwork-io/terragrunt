inputs = {
  name = "World"
}

terraform {
  source = "git::__MIRROR_SSH_URL__//test/fixtures/download/hello-world-no-remote?ref=v0.93.2"
}
