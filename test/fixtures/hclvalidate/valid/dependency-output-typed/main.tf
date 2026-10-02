variable "name_set" {
  type = set(string)
}

variable "name_count" {
  type = number
}

variable "has_names" {
  type = bool
}

variable "nested" {
  type = object({
    names = list(string)
  })
}
