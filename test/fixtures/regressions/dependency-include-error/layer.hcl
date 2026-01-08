# Layer config with dependency block and inputs referencing dependency outputs
# This triggers the false positive error when parsed via include directive

dependency "dep" {
  config_path = "${get_terragrunt_dir()}/../dep"
}

inputs = {
  # These lines reference dependency.dep.outputs which haven't been resolved yet
  # during include parsing, causing false positive "Unknown variable" errors
  dep_output_value = dependency.dep.outputs.value
}
