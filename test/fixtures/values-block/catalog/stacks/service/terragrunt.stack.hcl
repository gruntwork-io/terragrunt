values {
  value "name" {
    type = string
  }

  value "env" {
    type    = string
    default = "dev"
  }

  value "instance_count" {
    type    = number
    default = 2
  }
}

unit "app" {
  source = "${get_repo_root()}/catalog/units/app"
  path   = "app-${values.env}"

  values = {
    name           = values.name
    instance_count = values.instance_count
  }
}
