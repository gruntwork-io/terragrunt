output "rendered_template" {
  value = data.external.hello.result["output"]
}

