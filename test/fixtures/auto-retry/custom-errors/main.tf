resource "random_id" "filename" {
  byte_length = 8
}

resource "null_resource" "tf_retryable_error" {
  triggers = {
    always_recreate = timestamp()
  }

  provisioner "local-exec" {
    // The command will fail with a custom retryable error that matches the config the first time it's run,
    // and succeed on the 2nd run
    command     = "${path.module}/script.sh ${random_id.filename.hex}"
    interpreter = ["/bin/bash", "-c"]
  }
}
