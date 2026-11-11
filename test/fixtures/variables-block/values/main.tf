variable "number" {
  type = number
}

variable "large_number" {
  type = number
}

variable "flag" {
  type = bool
}

variable "object" {
  type = object({ name = string, sizes = list(number) })
}

variable "keyword_keys" {
  type = map(string)
}

variable "empty_list" {
  type = list(string)
}

variable "empty_map" {
  type = map(string)
}

variable "template" {
  type = string
}

variable "directive" {
  type = string
}

variable "multiple_lines" {
  type = string
}

variable "unicode" {
  type = string
}

output "values" {
  value = {
    number         = var.number
    large_number   = var.large_number
    flag           = var.flag
    object         = var.object
    keyword_keys   = var.keyword_keys
    empty_list     = var.empty_list
    empty_map      = var.empty_map
    template       = var.template
    directive      = var.directive
    multiple_lines = var.multiple_lines
    unicode        = var.unicode
  }
}
