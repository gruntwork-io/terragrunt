# Stack-level autoinclude: the autoinclude sits on a "stack" block, so it must
# generate terragrunt.autoinclude.stack.hcl (NOT the unit filename) in the
# generated nested-stack directory.
#
# Stacks do not have dependencies (only units do), so a stack autoinclude injects
# valid terragrunt.stack.hcl content. Here it patches the nested stack to add an
# "extra" unit.
stack "networking" {
  source = "../stacks/networking"
  path   = "networking"

  autoinclude {
    unit "extra" {
      source = "${get_repo_root()}/units/extra"
      path   = "extra"
    }
  }
}
