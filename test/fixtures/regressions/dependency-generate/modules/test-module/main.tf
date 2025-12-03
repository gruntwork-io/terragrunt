variable "token_via_input" {
  type = string
}

resource "local_file" "test" {
  filename = "./test-output.txt"
  content  = "Token via input: ${var.token_via_input}"
}
