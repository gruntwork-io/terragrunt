locals {
  roles = ["alpha", "beta", "gamma", "delta"]
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
