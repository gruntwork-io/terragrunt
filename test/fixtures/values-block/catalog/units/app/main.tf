variable "name" {
  type = string
}

variable "instance_count" {
  type = number
}

variable "tags" {
  type = map(string)
}

output "name" {
  value = var.name
}

output "instance_count" {
  value = var.instance_count
}

output "tags" {
  value = var.tags
}
