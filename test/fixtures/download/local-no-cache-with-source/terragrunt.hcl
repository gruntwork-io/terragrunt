terraform {
  no_cache = true
  source   = "./app"
}

inputs = {
  test_value = "no-cache-with-source-test"
}
