unit "app" {
  source = "../units/app"
  path   = "app"

  # Malformed on purpose: a locals block inside autoinclude is rejected by the
  # strict autoinclude parser, but the lenient stack decode leaves it in Remain.
  autoinclude {
    locals {
      env = "dev"
    }
  }
}
