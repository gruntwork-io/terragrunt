
include {
  path = find_in_parent_folders("eng_teams.hcl")
}

include {
  path = find_in_parent_folders("_envcommon.hcl")
}

inputs = {
  app = {
    "kind"      = "deployment",
    "namespace" = "foobar",
    "slug"      = "bar-foo",
    "data"      = "46521694",
  }
}
