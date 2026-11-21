unit "app" {
  source = "${dirname(path.include)}/units/app"
  path   = "${basename(dirname(path.include))}-app"

  values = {
    include   = path.include
    stack_dir = path.stack
  }

  autoinclude {
    inputs = {
      auto_include = path.include
      auto_label   = "${basename(path.include)}:${path.unit}"
    }
  }
}
