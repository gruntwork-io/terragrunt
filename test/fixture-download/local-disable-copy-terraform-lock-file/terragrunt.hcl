inputs = {
  name = "World"
}

terraform {
  source                   = "../hello-world"
  copy_terraform_lock_file = false
}
