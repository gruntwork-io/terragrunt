include "download" {
  path = find_in_parent_folders("download.hcl")
}

inputs = {
  module = path.module
}
