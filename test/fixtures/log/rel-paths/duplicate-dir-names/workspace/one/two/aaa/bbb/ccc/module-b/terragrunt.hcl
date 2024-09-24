dependency "databricks_workspace" {
  config_path  = "../workspace"
  skip_outputs = true
}

terraform {
  source = "../../../..//tf"
}
