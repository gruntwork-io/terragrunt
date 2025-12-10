
terraform {
  source            = "./modules/example"
  include_in_copy   = ["parent-include.txt"]
  exclude_from_copy = ["parent-exclude/**"]
}
