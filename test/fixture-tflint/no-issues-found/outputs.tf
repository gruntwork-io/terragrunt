output "aws_region" {
  description = "The AWS region's name."
  value       = var.aws_region
}
output "env" {
  description = "The randomized environment's name."
  value       = "${var.env}-${random_id.env.hex}"
}