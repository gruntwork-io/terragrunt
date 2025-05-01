locals {
  all_files        = split("\n", run_cmd("--terragrunt-quiet", "ls", "-p", "src"))
  all_files_marked = [for f in local.all_files : mark_as_read("src/${f}")]
}
