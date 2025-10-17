# Top-level stack that includes nested stacks
# This reproduces the structure from bug #4977

# Top-level units
unit "id" {
  source = "${path_relative_from_include()}/units/id"
  path   = "id"
}

unit "ecr-cache" {
  source = "${path_relative_from_include()}/units/ecr-cache"
  path   = "ecr-cache"
}

# Nested stacks
stack "network" {
  source = "${path_relative_from_include()}/stacks/network"
  path   = "network"
}

stack "k8s" {
  source = "${path_relative_from_include()}/stacks/k8s"
  path   = "k8s"
}
