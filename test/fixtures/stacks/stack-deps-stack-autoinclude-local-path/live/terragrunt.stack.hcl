# The "keep" unit carries an autoinclude so the phased autoinclude parser (and the override prune) runs.
# The sibling terragrunt.autoinclude.stack.hcl injects a unit whose path references local.region, which is
# defined here. Generation must read only the injected block names when pruning, so it must not fail trying
# to evaluate the injected path against an eval context that has no local.* populated.
locals {
  region = "eu"
}

unit "keep" {
  source = "${get_repo_root()}/units/keep"
  path   = "keep"

  autoinclude {
    inputs = {
      ok = "keep"
    }
  }
}
