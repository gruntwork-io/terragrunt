terraform {
  source = "../modules//app"
}

inputs = {
  module      = path.download
  working_dir = get_working_dir()
}
