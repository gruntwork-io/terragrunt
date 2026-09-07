dependency "aurora" {
  expansion {
    count = 24
  }

  config_path  = "../aurora-web"
  skip_outputs = true

  mock_outputs = {
    id = "instance-${count.index}"
  }
}

inputs = {
  ids = { for key, instance in dependency.aurora : key => instance.outputs.id }
}
