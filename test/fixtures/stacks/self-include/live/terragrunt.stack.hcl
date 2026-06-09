locals {
  version = "main"
}

unit "app1" {
  source = "git::__MIRROR_URL__//test/fixtures/stacks/self-include/unit?ref=${local.version}"
  path   = "app1"
  values = {
    data = "example-data"
  }
}


