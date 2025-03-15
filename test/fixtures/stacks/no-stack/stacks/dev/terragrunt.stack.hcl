unit "second-stack-unit-config" {
  source                  = "${get_repo_root()}/config"
  path                    = "second-stack-unit-config"
  no_dot_terragrunt_stack = true
}

unit "dev-api" {
  source = "${get_repo_root()}/units/api"
  path   = "api"
  values = {
    ver = "dev-api 1.0.0"
  }
}

unit "dev-db" {
  source = "${get_repo_root()}/units/db"
  path   = "db"
  values = {
    ver = "dev-db 1.0.0"
  }
}

unit "dev-web" {
  source = "${get_repo_root()}/units/web"
  path   = "web"
  values = {
    ver = "dev-web 1.0.0"
  }
}
