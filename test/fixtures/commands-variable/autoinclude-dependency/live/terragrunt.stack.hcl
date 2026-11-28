unit "apply" {
  source = "../../autoinclude/units/app"
  path   = "apply"
}

unit "app" {
  source = "../../autoinclude/units/app"
  path   = "app"

  autoinclude {
    dependency "apply" {
      config_path  = "../${commands.accept_parallelism[0]}"
      skip_outputs = true
    }
  }
}
