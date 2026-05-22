resource "local_file" "marker" {
  content  = "Hello from unit foo inside stack foo!"
  filename = "${path.module}/output.txt"
}

output "val" {
  value = "from-stack-foo-unit-foo"
}
