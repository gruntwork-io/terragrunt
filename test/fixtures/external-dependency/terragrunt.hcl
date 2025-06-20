feature "dep" {
  default = "/tmp/external"
}

dependencies {
  paths = [feature.dep.value]
}
