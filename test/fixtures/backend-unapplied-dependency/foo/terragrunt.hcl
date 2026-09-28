dependency "bar" {
  config_path = "../bar"
}

remote_state {
  backend = "local"

  config = {
    path = "foo.tfstate"
  }
}

inputs = {
  name = dependency.bar.outputs.name
}
