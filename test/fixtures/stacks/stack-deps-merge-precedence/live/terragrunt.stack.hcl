# The unit's own terragrunt.hcl sets inputs.val; the autoinclude sets the same
# key to a different value. The documented deep-merge has the autoinclude win.
unit "target" {
  source = "../catalog/units/target"
  path   = "target"

  autoinclude {
    inputs = {
      val = "from-autoinclude"
    }
  }
}
