terraform {
  no_dot_terragrunt_cache = true
}

# A null input is passed through a generated .terragrunt-null-vars.auto.tfvars.json file, which lands in the
# unit directory when the unit runs in place.
inputs = {
  var1 = null
  var2 = "variable 2"
}
