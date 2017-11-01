terragrunt = {
  terraform {
    extra_arguments "extra-from-parent" {
      commands  = ["plan", "apply"]
      arguments = ["-var", "foo_parent=${get_parent_tfvars_dir()}", "-var", "foo_child=${get_tfvars_dir()}", "-var", "foo_from_include=${path_relative_from_include()}", "-var", "foo_to_include=${path_relative_to_include()}"]
    }
  }
}
