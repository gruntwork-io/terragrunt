name = "Module B"

terragrunt = {
  terraform {
    source = "../../hello-world"
  }

  prevent_destroy = true

  include = {
    path = "${find_in_parent_folders("terraform.tfvars")}"
  }
}
