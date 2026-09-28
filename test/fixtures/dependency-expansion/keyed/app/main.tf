variable "vpc_id" {
  type = string
}

variable "web_id" {
  type = string
}

variable "api_id" {
  type = string
}

variable "shard_first" {
  type = string
}

variable "shard_second" {
  type = string
}

output "vpc_id" {
  value = var.vpc_id
}

output "web_id" {
  value = var.web_id
}

output "api_id" {
  value = var.api_id
}

output "shard_first" {
  value = var.shard_first
}

output "shard_second" {
  value = var.shard_second
}
