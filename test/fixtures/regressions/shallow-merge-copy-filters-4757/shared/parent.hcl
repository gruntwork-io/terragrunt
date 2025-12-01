# Parent config with terraform source but NO include_in_copy/exclude_from_copy
# This tests issue #4757 where shallow merge was dropping child's copy filters
terraform {
  source = "./modules/example"
  # intentionally NO include_in_copy or exclude_from_copy here
}