terraform {
  source = "../modules//app"
}

download_dir = "${get_terragrunt_dir()}/../custom-cache"

inputs = {
  module = path.download
}
