unit "db" {
  source = "../../autoinclude/units/db"
  path   = "db"
}

unit "app" {
  source = "../../autoinclude/units/app"
  path   = "app"

  autoinclude {
    dependency "db" {
      config_path  = "${path.stack}/.terragrunt-stack/db"
      skip_outputs = true
    }
  }
}
