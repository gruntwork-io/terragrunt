values {
  value "name" {
    type = string
  }

  value "tags" {
    type    = map(string)
    default = {}
  }
}

inputs = {
  name = values.name
  tags = try(values.tgas, {})
}
