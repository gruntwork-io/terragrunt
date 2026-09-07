locals {
  roles = ["alpha", "gamma"]
}

unit "aurora" {
  expansion {
    count = length(local.roles)
  }

  source = "../modules/app"
  path   = "aurora/${count.index}"

  values = {
    role = local.roles[count.index]
  }
}
