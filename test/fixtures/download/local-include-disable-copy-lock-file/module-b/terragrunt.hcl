inputs = {
  name = "Module B"
}

terraform {
  source = "../../hello-world"
  copy_terraform_lock_file = false
}

prevent_destroy = true

include {
  path = find_in_parent_folders("root.hcl")
}
