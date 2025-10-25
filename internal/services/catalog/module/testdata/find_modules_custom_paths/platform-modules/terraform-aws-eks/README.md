# EKS Platform Module

This Terraform Module provisions a complete Amazon EKS (Elastic Kubernetes Service) platform with managed node groups, cluster add-ons, and IRSA configuration.

## How does this work?

This is a high-level platform module that combines multiple EKS components:

- EKS cluster with configurable version
- Managed node groups with auto-scaling
- Essential cluster add-ons (VPC CNI, CoreDNS, kube-proxy)
- OIDC provider for IRSA
- Security groups and IAM roles
- CloudWatch logging

Perfect for teams that want a production-ready EKS setup without managing individual components.

## How do you use this module?
```hcl
module "eks_platform" {
  source = "git::https://github.com/example-org/infrastructure-modules.git//platform/terraform-aws-eks"

  cluster_name    = "production-platform"
  cluster_version = "1.28"

  vpc_id              = module.vpc.vpc_id
  private_subnet_ids  = module.vpc.private_subnet_ids
  public_subnet_ids   = module.vpc.public_subnet_ids

  node_groups = {
    general = {
      min_size     = 2
      max_size     = 10
      desired_size = 3
      instance_types = ["t3.large"]
    }
  }

  enable_irsa                = true
  enable_cluster_autoscaler  = true
  enable_metrics_server      = true

  tags = {
    Environment = "production"
    Platform    = "eks"
  }
}
```

## What's included?

- Fully configured EKS cluster
- Auto-scaling node groups
- Cluster authentication via IAM
- IRSA for pod-level permissions
- CloudWatch logging and monitoring
- Production-ready defaults

## Post-deployment
```bash
# Configure kubectl
aws eks update-kubeconfig --name production-platform --region us-east-1

# Verify cluster
kubectl get nodes
kubectl get pods -n kube-system
```
