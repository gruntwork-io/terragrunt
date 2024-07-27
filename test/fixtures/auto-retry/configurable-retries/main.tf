resource "null_resource" "tf_success_on_fifth_retry" {
  provisioner "local-exec" {
    interpreter = ["/bin/bash", "-c"]

    // The command will fail with 'TLS handshake timeout' error on the first
    // run, and on the fifth will succeed. We use a 'count' file to keep track
    // of the run number.
    command = <<EOF
if ! [ -f count ]; then i=1; else i="$((1 + $(cat count)))"; fi
echo "$i" > count
    
if [ "$i" -lt 5 ]; then
    echo 'Failed to load backend: Error configuring the backend 's3': RequestError: send request failed caused by: Post https://sts.amazonaws.com/: net/http: TLS handshake timeout'
    exit 1
else
    exit 0
fi
EOF
  }
}

variable "some_var" {
  description = "Just to get some output"
  default     = "Some value"
}

output "test" {
  value = var.some_var
}
