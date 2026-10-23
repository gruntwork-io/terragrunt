terraform {
  source   = "git::__MIRROR_URL__//test/fixtures/download/hello-world-no-remote?ref=v0.93.2"
  no_cache = true
}

inputs = {
  test_value = "should-error"
}
