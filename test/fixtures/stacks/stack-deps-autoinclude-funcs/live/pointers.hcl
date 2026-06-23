locals {
  # An absolute path to the generated data unit, read at stack generate time via read_terragrunt_config.
  data_config_path = "${get_repo_root()}/live/.terragrunt-stack/data"
}
