values {
  value "name" {
    type        = string
    description = "Name of the app."
  }

  value "instance_count" {
    type    = number
    default = 1
  }

  value "tags" {
    type    = map(string)
    default = {}
  }
}

inputs = {
  name           = values.name
  instance_count = values.instance_count
  tags           = values.tags
}
