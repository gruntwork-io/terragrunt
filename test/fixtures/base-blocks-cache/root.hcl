# The shared parent every unit includes. It calls only the functions classified as
# child-independent, so it is the file the base blocks cache is allowed to decode once and
# reuse for every unit. The functions that answer per unit are called by the units themselves.

locals {
  platform         = get_platform()
  from_env         = get_env("TG_FIXTURE_SHARED_VALUE", "fallback")
  need_vars        = get_terraform_commands_that_need_vars()
  need_locking     = get_terraform_commands_that_need_locking()
  need_input       = get_terraform_commands_that_need_input()
  need_parallelism = get_terraform_commands_that_need_parallelism()
  retryable        = get_default_retryable_errors()
  constraint_met   = constraint_check("1.2.3", ">= 1.0.0")
  merged           = deep_merge({ left = "one" }, { right = "two" })
  starts_with      = startswith("terragrunt", "terra")
  ends_with        = endswith("terragrunt", "grunt")
  contains         = strcontains("terragrunt", "agr")
  compared         = timecmp("2026-01-01T00:00:00Z", "2026-06-01T00:00:00Z")
}

inputs = {
  platform         = local.platform
  from_env         = local.from_env
  need_vars        = local.need_vars
  need_locking     = local.need_locking
  need_input       = local.need_input
  need_parallelism = local.need_parallelism
  retryable        = local.retryable
  constraint_met   = local.constraint_met
  merged           = local.merged
  starts_with      = local.starts_with
  ends_with        = local.ends_with
  contains         = local.contains
  compared         = local.compared
}
