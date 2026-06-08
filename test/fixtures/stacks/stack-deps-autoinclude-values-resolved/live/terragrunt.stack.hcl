unit "app" {
  source = "${get_repo_root()}/units/app"
  path   = "app"

  autoinclude {
    inputs = {
      # values.* resolves against the stack file's terragrunt.values.hcl at stack generate time.
      region = values.region
      # A directory function resolves in the stack file's context at generate time, not the generated unit's.
      stack_dir = get_terragrunt_dir()
    }
  }
}
