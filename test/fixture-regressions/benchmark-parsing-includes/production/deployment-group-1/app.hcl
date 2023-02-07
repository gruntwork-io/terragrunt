locals {
  current_dir = get_terragrunt_dir()
  name        = basename(local.current_dir)
}
