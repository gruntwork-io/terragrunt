# Parent config with terraform source AND copy filters defined
# This tests that child values override parent values in shallow merge
terraform {
  source            = "./modules/example"
  include_in_copy   = ["parent-include.txt"]
  exclude_from_copy = ["parent-exclude/**"]
}