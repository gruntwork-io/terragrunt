resource "terraform_data" "dep" {
  input = "dep"
}

output "value" {
  value = terraform_data.dep.input
}
