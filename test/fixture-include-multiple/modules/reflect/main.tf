variable "attribute" {
  type = string
}

variable "new_attribute" {
  type = string
}

variable "old_attribute" {
  type = string
}

variable "list_attr" {
  type = list(string)
}

variable "map_attr" {
  type = map(string)
}

variable "dep_out" {
  type = any
}

output "attribute" {
  value = var.attribute
}

output "new_attribute" {
  value = var.new_attribute
}

output "old_attribute" {
  value = var.old_attribute
}

output "list_attr" {
  value = var.list_attr
}

output "map_attr" {
  value = var.map_attr
}

output "dep_out" {
  value = var.dep_out
}
