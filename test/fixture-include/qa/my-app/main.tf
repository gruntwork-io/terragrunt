# Create an arbitrary local resource
data "external" "hello" {
  program = ["jq", "-n", "{\"greeting\": \"Hello, I am a template.\"}"]
}

variable "reflect" {
  type = string
}

output "reflect" {
  value = var.reflect
}

output "mock" {
  value = data.external.hello.result.greeting
}

