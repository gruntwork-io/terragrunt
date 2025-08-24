include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "../../catalog/modules//best_cat"
}

inputs = {
  name = "best-cat-2025-07-31-01-dev"

  lambda_zip_file = "${get_repo_root()}/dist/best-cat.zip"
}
