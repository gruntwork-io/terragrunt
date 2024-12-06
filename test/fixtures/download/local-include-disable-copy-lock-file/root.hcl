terraform {
  include_in_copy = ["**/.terraform-version"]
}

dependencies {
  paths = ["../module-a"]
}