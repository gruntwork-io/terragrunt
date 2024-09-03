output "text" {
  value = jsondecode(file("${path.module}/data.json"))["text"]
}
