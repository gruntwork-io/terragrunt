# This config intentionally sets terraform_binary to a path that doesn't exist.
# If the --tf-path CLI argument is properly respected, this should be overridden.
terraform_binary = "./non-existent"

dependency "dep" {
  config_path = "../dep"
}

inputs = {
  dep_value = dependency.dep.outputs.value
}
