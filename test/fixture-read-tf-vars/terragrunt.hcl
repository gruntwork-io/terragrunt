locals {
  vars       = jsondecode(read_tfvars_file("my.tfvars"))
  json_vars = jsondecode(read_tfvars_file("my.tfvars.json"))
  string_var = local.vars.string_var
  bool_var   = local.vars.bool_var
  number_var = local.vars.number_var
  list_var   = local.vars.list_var

  json_string_var = local.json_vars.string_var
  json_bool_var   = local.json_vars.bool_var
  json_number_var = local.json_vars.number_var
}
