feature "dep" {
  default = "../external-dependencies/module-a"
}

dependencies {
  paths = [feature.dep.value]
}
