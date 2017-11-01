terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }

  terraform {
    extra_arguments "extra-from-child" {
      commands  = ["plan", "apply"]
      arguments = ["-var", "bar_parent=${get_parent_tfvars_dir()}", "-var", "bar_child=${get_tfvars_dir()}", "-var", "bar_from_include=${path_relative_from_include()}", "-var", "bar_to_include=${path_relative_to_include()}"]
    }
  }
}
