# This fixture is designed to cause an error to test tip display
terraform {
  source = "./non-existent-module"
}
