dependency "a" {
  config_path = "../a"
}

dependency "b" {
  config_path = "../b"
}

inputs = {
  test_a_arn = dependency.a.outputs.value
  test_b_arn = dependency.b.outputs.value
}

