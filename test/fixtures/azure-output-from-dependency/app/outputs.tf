output "received_foo" {
  value       = var.vpc_config.foo
  description = "Foo value received from dependency"
}
