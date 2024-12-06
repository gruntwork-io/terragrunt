include {
  path = "${find_in_parent_folders("root.hcl")}"
}

dependencies {
  paths = ["../app3", "../app1"]
}
