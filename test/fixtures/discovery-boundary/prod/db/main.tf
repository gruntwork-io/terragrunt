resource "null_resource" "db" {
  triggers = {
    name = "db"
  }
}

output "db_id" {
  value = "db-12345"
}
