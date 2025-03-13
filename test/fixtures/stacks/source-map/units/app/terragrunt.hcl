terraform {
  source = "git::https://git-host.com/not-existing-repo.git//fixtures/stacks/source-map/tf/modules"
}

inputs = {
  input = values.input
}