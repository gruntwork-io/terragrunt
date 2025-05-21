# ALB Ingress Controller IAM Policy Module

This Terraform Module defines an [IAM
policy](http://docs.aws.amazon.com/AmazonCloudWatch/latest/DeveloperGuide/QuickStartEC2Instance.html#d0e22325) that
defines the minimal set of permissions necessary for the [AWS ALB Ingress
Controller](https://github.com/kubernetes-sigs/aws-alb-ingress-controller). This policy can then be attached to EC2
instances or IAM roles so that the controller deployed has enough permissions to manage an ALB.

See [the eks-alb-ingress-controller module](/modules/eks-alb-ingress-controller) for a module that deploys the Ingress
Controller on to your EKS cluster.


## How do you use this module?

* See the [root README](/README.adoc) for instructions on using Terraform modules.
* See the [eks-cluster-with-supporting-services example](/examples/eks-cluster-with-supporting-services) for example
  usage.
* See [variables.tf](./variables.tf) for all the variables you can set on this module.
* See [outputs.tf](./outputs.tf) for all the variables that are outputted by this module.


## Attaching IAM policy to workers

To allow the ALB Ingress Controller to manage ALBs, it needs IAM permissions to use the AWS API to manage ALBs.
Currently, the way to grant Pods IAM privileges is to use the worker IAM profiles provisioned by [the
eks-cluster-workers module](/modules/eks-cluster-workers/README.md#how-do-you-add-additional-iam-policies).

The Terraform templates in this module create an IAM policy that has the required permissions. You then need to use an
[aws_iam_policy_attachment](https://www.terraform.io/docs/providers/aws/r/iam_policy_attachment.html) to attach that
policy to the IAM roles of your EC2 Instances.

```hcl
module "eks_workers" {
  # (arguments omitted)
}

module "alb_ingress_controller_iam_policy" {
  # (arguments omitted)
}

resource "aws_iam_role_policy_attachment" "attach_alb_ingress_controller_iam_policy" {
    role = "${module.eks_workers.eks_worker_iam_role_name}"
    policy_arn = "${module.alb_ingress_controller_iam_policy.alb_ingress_controller_policy_arn}"
}
```
