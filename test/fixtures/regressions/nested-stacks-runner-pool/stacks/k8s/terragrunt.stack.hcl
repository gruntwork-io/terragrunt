
unit "eks-cluster" {
  path   = "eks-cluster"
  source = "${get_repo_root()}/_source/units/eks-cluster"
  values = values
}

unit "eks-baseline" {
  path   = "eks-baseline"
  source = "${get_repo_root()}/_source/units/eks-baseline"
  values = values
}

unit "grafana-baseline" {
  path   = "grafana-baseline"
  source = "${get_repo_root()}/_source/units/grafana-baseline"
  values = values
}

unit "rancher-baseline" {
  path   = "rancher-baseline"
  source = "${get_repo_root()}/_source/units/rancher-baseline"
  values = values
}

unit "rancher-bootstrap" {
  path   = "rancher-bootstrap"
  source = "${get_repo_root()}/_source/units/rancher-bootstrap"
  values = values
}


