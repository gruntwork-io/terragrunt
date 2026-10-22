# The "keep" unit carries an autoinclude so the phased autoinclude parser runs.
# The sibling terragrunt.autoinclude.stack.hcl injects a unit whose path references local.region, which is
# defined here. Generation must evaluate the injected path against this file's locals.
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
