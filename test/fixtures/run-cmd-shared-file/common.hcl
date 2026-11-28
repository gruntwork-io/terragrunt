# Shared by every unit via read_terragrunt_config, so each run_cmd below is
# evaluated under this file's directory for all of them. The calls are quiet so
# their stdout does not pollute `output -json`.
locals {
  account = run_cmd("--terragrunt-quiet", "${get_terragrunt_dir()}/print-account.sh")
  joined  = run_cmd("--terragrunt-quiet", "printf", "%s|", "x y")
  split   = run_cmd("--terragrunt-quiet", "printf", "%s|", "x", "y")
}
