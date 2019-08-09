dependencies {
  paths = ["../app2", "../app1"]
}

inputs = {
  x = tonumber(trimspace(run_cmd("terragrunt", "output", "x", "--terragrunt-working-dir", "${get_terragrunt_dir()}/../app1")))
  y = tonumber(trimspace(run_cmd("terragrunt", "output", "y", "--terragrunt-working-dir", "${get_terragrunt_dir()}/../app2")))
  # x = get_output("../app1", "x")
  # y = get_output("../app2", "y")
}
