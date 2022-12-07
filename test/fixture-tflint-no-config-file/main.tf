provider "aws" {
  region = "eu-west-1"
}

variable "instance_class" {
  type = string
}

resource "aws_db_instance" "default" {
  allocated_storage   = 10
  db_name             = "mydb"
  engine              = "mysql"
  engine_version      = "5.7"
  instance_class      = var.instance_class
  username            = "foo"
  password            = "foobarbaz"
  skip_final_snapshot = true
}
