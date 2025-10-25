# Security Group Module

This Terraform Module creates and manages AWS Security Groups with configurable ingress and egress rules for controlling network access to resources.

## How does this work?

Security Groups act as virtual firewalls controlling inbound and outbound traffic. This module:

- Creates a security group with descriptive name and tags
- Configures ingress rules for inbound traffic
- Configures egress rules for outbound traffic
- Supports referencing other security groups
- Allows CIDR-based and security group-based rules

## How do you use this module?
```hcl
module "web_security_group" {
  source = "git::https://github.com/example-org/infrastructure-modules.git//infrastructure/terraform-aws-security-group"

  name        = "web-server-sg"
  description = "Security group for web servers"
  vpc_id      = module.vpc.vpc_id

  ingress_rules = [
    {
      from_port   = 443
      to_port     = 443
      protocol    = "tcp"
      cidr_blocks = ["0.0.0.0/0"]
      description = "HTTPS from anywhere"
    },
    {
      from_port   = 80
      to_port     = 80
      protocol    = "tcp"
      cidr_blocks = ["0.0.0.0/0"]
      description = "HTTP from anywhere"
    }
  ]

  egress_rules = [
    {
      from_port   = 0
      to_port     = 0
      protocol    = "-1"
      cidr_blocks = ["0.0.0.0/0"]
      description = "Allow all outbound"
    }
  ]

  tags = {
    Environment = "production"
  }
}
```

## Security Best Practices

- Follow principle of least privilege
- Use specific ports instead of wide ranges
- Reference security groups instead of CIDR blocks when possible
- Always add descriptions to rules
- Regularly audit and remove unused rules
