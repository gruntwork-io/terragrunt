unit "vpc" {
  source = "${get_repo_root()}/units/vpc"
  path   = "vpc"
}

unit "private" {
  enabled = false

  source = "${get_repo_root()}/units/subnet"
  path   = "private"

  values = {
    name = "private"
  }
}

unit "subnet" {
  expansion {
    count = 3
  }

  enabled = count.index != 1

  source = "${get_repo_root()}/units/subnet"
  path   = "subnet-${count.index}"

  values = {
    name = "subnet-${count.index}"
  }
}

stack "legacy" {
  enabled = false

  source = "${get_repo_root()}/stacks/legacy"
  path   = "legacy"
}
