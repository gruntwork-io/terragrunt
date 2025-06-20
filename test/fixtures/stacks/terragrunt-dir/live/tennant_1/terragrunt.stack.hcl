unit "unit_a" {
  source = "../../unit_a"
  path   = "unit_a"
  values = {
    terragrunt_dir = get_terragrunt_dir()
  }
}