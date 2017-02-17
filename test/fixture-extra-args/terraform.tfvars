terragrunt= {
  terraform = {
    extra_arguments "test" {
      arguments = [
        "-var-file=terraform.tfvars",
        "-var-file=extra.tfvars"
      ]
      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh"
      ]
    }
  }
}
