output "name" {
  value = aws_lambda_function.main.function_name
}

output "arn" {
  value = aws_lambda_function.main.arn
}

output "url" {
  value = aws_lambda_function_url.main.function_url
}
