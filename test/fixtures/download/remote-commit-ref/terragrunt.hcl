inputs = {
  name = "World"
}

terraform {
  # Pinned to the commit v0.93.2 resolves to so this fixture exercises
  # the commit-SHA path in the CAS getter without depending on whatever
  # the tag points to in the future.
  source = "github.com/gruntwork-io/terragrunt.git//test/fixtures/download/hello-world-no-remote?ref=dd7913e04f0e812b51ccf2c4f35a0fda16a356a1"
}
