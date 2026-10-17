dependency "dep" {
  config_path = "../dep"
}

inputs = {
  x = concat(["static"], dependency.dep.outputs.ips)
}
