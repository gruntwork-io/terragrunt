locals {
  module = path.module
}

inputs = {
  module      = local.module
  working_dir = get_working_dir()
  whole       = path
}
