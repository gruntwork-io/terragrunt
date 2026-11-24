dependency "local" {
  config_path = "../../local"
}

download_dir = dependency.local.outputs.module

inputs = {
  module = path.download
}
