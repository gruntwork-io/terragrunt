terraform {
  source = "${get_terragrunt_dir()}/."

  extra_arguments "broken" {
    commands  = ["plan", "apply"]
    arguments = [nonexistent_function_xyz("boom")]
  }
}
