variable "string_feature_flag" {
  type = string
}

variable "int_feature_flag" {
  type = number
}

variable "bool_feature_flag" {
  type = bool
}

output "string_feature_flag" {
  value = var.string_feature_flag
}

output "int_feature_flag" {
  value = var.int_feature_flag
}

output "bool_feature_flag" {
  value = var.bool_feature_flag
}
