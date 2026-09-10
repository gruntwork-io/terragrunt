# A shared parent calling only the functions classified as answering per unit. The cache is
# meant to refuse this file outright: were it ever reused, the second unit to include it would
# be handed the first unit's directory, plan file and read set, and the rendered output would
# say so.

locals {
  relative_to_include   = path_relative_to_include()
  relative_from_include = path_relative_from_include()
  terragrunt_dir        = get_terragrunt_dir()
  original_dir          = get_original_terragrunt_dir()
  parent_dir            = get_parent_terragrunt_dir()
  source_cli_flag       = get_terragrunt_source_cli_flag()
  command               = get_terraform_command()
  cli_args              = get_terraform_cli_args()
  working_dir           = get_working_dir()
  repo_root             = get_repo_root()
  path_from_repo_root   = get_path_from_repo_root()
  path_to_repo_root     = get_path_to_repo_root()
  from_cmd              = run_cmd("--terragrunt-quiet", "cat", "marked.txt")
  from_config           = read_terragrunt_config("target.hcl").locals.value
  from_tfvars           = read_tfvars_file("vars.tfvars")
  marked                = mark_as_read("marked.txt")
  marked_glob           = mark_glob_as_read("*.txt")
}

inputs = {
  relative_to_include   = local.relative_to_include
  relative_from_include = local.relative_from_include
  terragrunt_dir        = local.terragrunt_dir
  original_dir          = local.original_dir
  parent_dir            = local.parent_dir
  source_cli_flag       = local.source_cli_flag
  command               = local.command
  cli_args              = local.cli_args
  working_dir           = local.working_dir
  repo_root             = local.repo_root
  path_from_repo_root   = local.path_from_repo_root
  path_to_repo_root     = local.path_to_repo_root
  from_cmd              = local.from_cmd
  from_config           = local.from_config
  from_tfvars           = local.from_tfvars
  marked                = local.marked
  marked_glob           = local.marked_glob
}
