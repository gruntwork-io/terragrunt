resource "null_resource" "tf_retryable_error" {
  provisioner "local-exec" {
    // The command will fail with 'TLS handshake timeout' error on the first run as the 'touched' file does not exist
    // On second pass the file is found and return code will be 0
    command     = "if [[ -f touched ]]; then exit 0; else echo 'Failed to load backend: Error configuring the backend 's3': RequestError: send request failed caused by: Post https://sts.amazonaws.com/: net/http: TLS handshake timeout' && touch touched && exit 1; fi"
    interpreter = ["/bin/bash", "-c"]
  }
}

output "app3_text" {
  value = "app3 output"
}
