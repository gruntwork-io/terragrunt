# Root configuration used by child configs via include
locals {
  root_value = "root-config-value"
}

inputs = {
  from_root = local.root_value
}
