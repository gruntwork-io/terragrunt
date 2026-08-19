terraform_binary = "./dependency-output-must-not-run"

include "root" {
  path = find_in_parent_folders("root.hcl")
}

