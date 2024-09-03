variable "string" {
  type = string
}

output "string" {
  value = var.string
}

variable "number" {
  type = number
}

output "number" {
  value = var.number
}

variable "bool" {
  type = bool
}

output "bool" {
  value = var.bool
}

variable "list_string" {
  type = list(string)
}

output "list_string" {
  value = var.list_string
}

variable "list_number" {
  type = list(number)
}

output "list_number" {
  value = var.list_number
}

variable "list_bool" {
  type = list(bool)
}

output "list_bool" {
  value = var.list_bool
}

variable "map_string" {
  type = map(string)
}

output "map_string" {
  value = var.map_string
}

variable "map_number" {
  type = map(number)
}

output "map_number" {
  value = var.map_number
}

variable "map_bool" {
  type = map(bool)
}

output "map_bool" {
  value = var.map_bool
}

variable "object" {
  type = object({
    str  = string
    num  = number
    list = list(number)
    map  = map(string)
  })
}

output "object" {
  value = var.object
}

variable "from_env" {
  type = string
}

output "from_env" {
  value = var.from_env
}