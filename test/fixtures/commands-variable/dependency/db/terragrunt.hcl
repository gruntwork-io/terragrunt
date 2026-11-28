locals {
  parallelism = commands.accept_parallelism
}

inputs = {
  parallelism = local.parallelism
}
