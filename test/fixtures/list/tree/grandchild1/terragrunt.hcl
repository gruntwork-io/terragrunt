include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "hashicorp/aws//examples/hello-world-app"
}
