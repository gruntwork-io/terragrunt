unit "app" {
  source = "${get_repo_root()}/units/app"
  path   = "app"

  autoinclude {
    inputs = {
      # values.* is a unit-scoped namespace and is rejected at stack generate time.
      region = values.region
    }
  }
}
