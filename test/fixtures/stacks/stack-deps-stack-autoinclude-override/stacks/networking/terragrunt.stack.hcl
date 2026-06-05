# The nested stack declares its own "vpc" unit sourced from vpc-base, carrying a nested unit-level
# autoinclude. The parent stack autoinclude injects a same-name "vpc" unit sourced from vpc-override,
# which must override this base block wholesale: the override source wins AND the base block's nested
# autoinclude must NOT leak into the generated overridden unit.
#
# The "sibling" unit is NOT overridden and keeps its own autoinclude. It exists so the post-override
# config still reports an autoinclude, which drives the phased autoinclude parser over the base file
# bytes. That base parse still sees vpc's autoinclude, so without pruning the override the base block's
# autoinclude would leak into the overridden vpc unit.
unit "vpc" {
  source = "${get_repo_root()}/units/vpc-base"
  path   = "vpc"

  autoinclude {
    inputs = {
      leaked = "base-leak"
    }
  }
}

unit "sibling" {
  source = "${get_repo_root()}/units/sibling"
  path   = "sibling"

  autoinclude {
    inputs = {
      ok = "sibling-ok"
    }
  }
}
