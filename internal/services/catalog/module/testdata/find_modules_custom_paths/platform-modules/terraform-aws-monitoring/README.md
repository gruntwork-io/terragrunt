# Monitoring Platform Module

This Terraform Module sets up a comprehensive monitoring and observability stack for AWS infrastructure using CloudWatch, SNS, and optional third-party integrations.

## How does this work?

This module provides a complete monitoring solution:

- CloudWatch dashboards for key metrics
- CloudWatch alarms for critical resources
- SNS topics for alert notifications
- Log aggregation and filtering
- Custom metric namespaces
- Optional integration with Datadog, Grafana, or Prometheus

## How do you use this module?
```hcl
module "monitoring" {
  source = "git::https://github.com/example-org/terraform-modules.git//platform/terraform-aws-monitoring"

  environment = "production"
  
  # SNS notification endpoints
  alert_email_addresses = [
    "ops-team@company.com",
    "oncall@company.com"
  ]

  # Resources to monitor
  monitored_resources = {
    albs = [module.alb.arn]
    eks_clusters = [module.eks.cluster_name]
    rds_instances = [module.database.instance_id]
  }

  # Alarm thresholds
  alb_target_response_time_threshold = 2.0
  eks_node_cpu_threshold             = 80
  rds_cpu_threshold                  = 75

  # Dashboard configuration
  create_overview_dashboard = true
  dashboard_time_range      = "1h"

  tags = {
    Environment = "production"
    Team        = "platform"
  }
}
```

## What resources does this module create?

- CloudWatch Dashboards
- CloudWatch Alarms
- SNS Topics and Subscriptions
- CloudWatch Log Groups
- Metric Filters
- EventBridge Rules (optional)

## Monitoring Coverage

### Infrastructure
- EC2 instance health and performance
- EKS cluster and node metrics
- RDS database metrics
- ALB/NLB request metrics

### Application
- Custom application metrics
- Log-based metrics
- API Gateway metrics
- Lambda function metrics

## Alert Severity Levels

- **Critical**: Immediate action required (page on-call)
- **Warning**: Investigation needed (email/Slack)
- **Info**: Informational (dashboard only)

## Integration Options

Supports integration with:
- PagerDuty
- Slack
- Datadog
- Grafana Cloud
- Prometheus
