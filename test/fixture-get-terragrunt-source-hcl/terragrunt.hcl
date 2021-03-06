inputs = {
  terragrunt_source = get_terragrunt_source()
}

terraform {
  source = "./terraform_config_hcl"
}
