values {
  value "source" {
    type    = string
    default = "."
  }

  value "net_path" {
    type = string
  }

  value "skip_destroy" {
    type    = bool
    default = false
  }
}

terraform {
  source = values.source
}

dependency "net" {
  config_path  = values.net_path
  mock_outputs = { id = "mock-id" }
}

exclude {
  if      = values.skip_destroy
  actions = ["destroy"]
}

inputs = {
  net_id = dependency.net.outputs.id
}
