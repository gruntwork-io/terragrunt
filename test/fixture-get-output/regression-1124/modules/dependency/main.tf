resource "random_string" "random" {
  length = 16
}

output "foo" {
  value = random_string.random.result
}
