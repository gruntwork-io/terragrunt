locals {
  module = path.download
}

inputs = {
  module      = local.module
  working_dir = get_working_dir()
  whole       = path
}
