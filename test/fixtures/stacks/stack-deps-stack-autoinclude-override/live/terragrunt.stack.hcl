# Stack-level autoinclude override: the autoinclude injects a "vpc" unit whose name
# matches the nested stack's own "vpc" unit. The injected block must override the base
# unit wholesale (source becomes vpc-override), while the new-name "added" unit is appended.
stack "networking" {
  source = "../stacks/networking"
  path   = "networking"

  autoinclude {
    unit "vpc" {
      source = "${get_repo_root()}/units/vpc-override"
      path   = "vpc"
    }

    unit "added" {
      source = "${get_repo_root()}/units/added"
      path   = "added"
    }
  }
}
