terraform {
  source = "test"
}

include {
  path = find_in_parent_folders()
}

dependencies {
  paths = ["../../module-a", "../../module-b/module-b-child"]
}
