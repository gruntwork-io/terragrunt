terraform {
  source = "git::ssh://git@github.com/gruntwork-io/i-dont-exist.git//fixtures/exec-cmd-source-map/module?ref=master"
}

inputs = {
  name = "terragrunt"
}
