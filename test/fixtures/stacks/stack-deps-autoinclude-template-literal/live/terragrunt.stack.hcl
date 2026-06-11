unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"
}

unit "app" {
  source = "../units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path
    }

    inputs = {
      # ${0} interpolates a number literal; the dependency.* ref forces the structural partial-eval path.
      v = "${0}-${dependency.vpc.outputs.id}"
    }
  }
}
