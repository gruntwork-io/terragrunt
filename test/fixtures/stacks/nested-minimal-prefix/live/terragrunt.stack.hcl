unit "vpc" {
  source = "${get_repo_root()}/units/null"
  path   = "vpc"
}

stack "app-a" {
  source = "${get_repo_root()}/stacks/app-a"
  path   = "app-a"
}

stack "app-b" {
  source = "${get_repo_root()}/stacks/app-b"
  path   = "app-b"
}
