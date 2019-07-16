inputs = {
  name = "Module B"
}

terraform {
  source = "../../hello-world"
}

prevent_destroy = true

include {
  path = find_in_parent_folders("terragrunt.hcl")
}
