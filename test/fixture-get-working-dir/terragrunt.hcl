terraform {
  source = "${local.source_base_url}/a"
}

locals {
  source_base_url = "modules"
}

inputs = {
  working_dir = get_working_dir()
}
