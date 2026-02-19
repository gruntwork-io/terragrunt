locals {
  my_value = local.missing_var  # Undefined - causes "value is not known" error
}

terraform {
  source = local.my_value  # Uses undefined value, source cannot be determined
}
