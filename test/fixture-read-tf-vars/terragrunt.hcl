locals {
  vars       = jsondecode(read_tfvars_file("my.tfvars"))
  string_var = local.vars.string_var
  bool_var   = local.vars.bool_var
  number_var = local.vars.number_var
  list_var   = local.vars.list_var
}
