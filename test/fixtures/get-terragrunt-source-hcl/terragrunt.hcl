inputs = {
  terragrunt_source = get_terragrunt_source_cli_flag()
}

terraform {
  source = "./terraform_config_hcl"
}
