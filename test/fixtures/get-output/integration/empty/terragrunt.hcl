# Intentionally only has one dependency with skip_outputs to test logic that it doesn't attempt to pull the outputs.
dependency "app1" {
  config_path = "../app1"
  skip_outputs = true
}
