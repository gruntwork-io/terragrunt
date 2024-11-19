
feature "exclude" {
    default = false
}

feature "exclude_dependencies" {
  default = false
}

exclude {
  if = feature.exclude.value
  actions = ["all"]
  exclude_dependencies = feature.exclude_dependencies.value
}

dependency "dep" {
  config_path = "../dep"

  mock_outputs = {
    data1 = "mock"
  }

}
