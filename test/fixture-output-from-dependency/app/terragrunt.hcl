dependencies {
  paths = ["../dependency"]
}

dependency "test" {
  config_path = "../dependency"
}

inputs = {
  vpc_config = dependency.test.outputs
}
