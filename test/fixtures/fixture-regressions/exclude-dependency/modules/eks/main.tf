resource "random_string" "random" {
  length = 16
}

output "random_string" {
  value = random_string.random.result
}
