
feature "exclude" {
    default = false
}

feature "exclude_dependents" {
  default = false
}

exclude {
  if = feature.exclude.value
  actions = ["all"]
  exclude_dependents = feature.exclude_dependents.value
}

dependency "dep" {
  config_path = "../dep"

  mock_outputs = {
    data1 = "mock"
  }

}
