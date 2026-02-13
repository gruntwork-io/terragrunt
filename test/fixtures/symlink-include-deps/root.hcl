# Root configuration used by child configs via include
locals {
  root_value = "from-root-config"
}

inputs = {
  from_root = local.root_value
}
