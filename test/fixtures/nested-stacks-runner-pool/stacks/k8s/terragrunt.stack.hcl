# K8s nested stack with multiple units
# This is where the bug manifests - units after eks-cluster are not included in runner pool

# Add dependency on network stack - this might trigger the bug
dependencies {
  paths = ["../network"]
}

unit "eks-cluster" {
  source = "${path_relative_from_include()}/../../units/eks-cluster"
  path   = "eks-cluster"
}

unit "eks-baseline" {
  source = "${path_relative_from_include()}/../../units/eks-baseline"
  path   = "eks-baseline"
}

unit "grafana-baseline" {
  source = "${path_relative_from_include()}/../../units/grafana-baseline"
  path   = "grafana-baseline"
}

unit "rancher-bootstrap" {
  source = "${path_relative_from_include()}/../../units/rancher-bootstrap"
  path   = "rancher-bootstrap"
}

unit "rancher-baseline" {
  source = "${path_relative_from_include()}/../../units/rancher-baseline"
  path   = "rancher-baseline"
}
