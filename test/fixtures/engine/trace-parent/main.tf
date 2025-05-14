data "external" "traceparent" {
  program = ["${path.module}/get_traceparent.sh"]

  query = {
    nonce = timestamp()
  }
}

output "traceparent_value" {
  value = data.external.traceparent.result["traceparent"]
}
