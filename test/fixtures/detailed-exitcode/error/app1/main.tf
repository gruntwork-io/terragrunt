data "local_file" "read_not_existing_file" {
  filename = "${path.module}/not-existing-file.txt"
}
