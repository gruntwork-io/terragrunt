variabl "d" {
  type    = string
  default = "d"
}

output "d" {
  value = var.d
}




By default terragrunt-format flag could have the value "%path:\%errros"

`terragurnt hclvalidate`
----
path/to/config/1:

Error: Unsupported block type
   on main.tf line 1:
    1: variabl "d" {
Blocks of type "variabl" are not expected here. Did you mean "variable"?

Error: Unsupported block type
   on main.tf line 1:
    1: variabl "d" {
Blocks of type "variabl" are not expected here. Did you mean "variable"?

path/to/config/2:

Error: Unsupported block type
   on main.tf line 1:
    1: variabl "d" {
Blocks of type "variabl" are not expected here. Did you mean "variable"?

Error: Unsupported block type
   on main.tf line 1:
    1: variabl "d" {
Blocks of type "variabl" are not expected here. Did you mean "variable"?
