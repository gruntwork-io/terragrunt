resource "null_resource" "tf_retryable_error" {
  provisioner "local-exec" {
    // The command will fail with a known retryable error 'TLS handshake timeout' error on all executions
    command     = "echo 'Failed to load backend: Error configuring the backend 's3': RequestError: send request failed caused by: Post https://sts.amazonaws.com/: net/http: TLS handshake timeout' && exit 1"
    interpreter = ["/bin/bash", "-c"]
  }
}

variable "some_var" {
  description = "Just to get some output"
  default     = "Some value"
}

output "test" {
  value = var.some_var
}
