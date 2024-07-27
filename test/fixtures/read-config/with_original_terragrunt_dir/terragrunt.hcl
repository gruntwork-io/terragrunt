locals {
  bar = read_terragrunt_config("foo/bar.hcl")
}

dependency "dep" {
  config_path = "./dep"
}

inputs = {
  terragrunt_dir          = local.bar.locals.terragrunt_dir
  original_terragrunt_dir = local.bar.locals.original_terragrunt_dir

  dep_terragrunt_dir          = dependency.dep.outputs.terragrunt_dir
  dep_original_terragrunt_dir = dependency.dep.outputs.original_terragrunt_dir

  dep_bar_terragrunt_dir          = dependency.dep.outputs.bar_terragrunt_dir
  dep_bar_original_terragrunt_dir = dependency.dep.outputs.bar_original_terragrunt_dir
}