terraform {
  source = "git::__MIRROR_URL__//test/fixtures/stacks/self-include/module?ref=main"
}

inputs = {
  data = values.data
}