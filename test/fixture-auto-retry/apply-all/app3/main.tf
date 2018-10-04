resource "null_resource" "tf_retryable_error" {
  provisioner "local-exec" {
    command = "if [[ -f touched ]]; then exit 0; else echo 'Failed to load backend: Error configuring the backend 's3': RequestError: send request failed caused by: Post https://sts.amazonaws.com/: net/http: TLS handshake timeout' && touch touched && exit 1; fi"
    interpreter = ["/bin/bash", "-c"]
  }
}

output "app3_text" {
  value = "app3 output"
}
