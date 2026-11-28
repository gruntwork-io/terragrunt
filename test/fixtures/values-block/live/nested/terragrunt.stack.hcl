stack "service" {
  source = "${get_repo_root()}/catalog/stacks/service"
  path   = "service"

  values = {
    name = "api"
  }
}
