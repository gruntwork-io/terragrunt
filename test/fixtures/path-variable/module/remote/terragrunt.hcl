terraform {
  source = "../modules//app"
}

inputs = {
  module      = path.module
  working_dir = get_working_dir()
}
