## What is the aws-auth-merger?

The `aws-auth-merger` is a go CLI intended to be run inside a Pod in an EKS cluster (as opposed to a CLI tool used by the
operator) for managing mappings between AWS IAM roles and users to RBAC groups in Kubernetes, and is an alternative to
[the official way AWS recommends managing the
mappings](https://docs.aws.amazon.com/eks/latest/userguide/add-user-role.html).
The official way to manage the mapping is to add values in a single, central `ConfigMap`. This central `ConfigMap` has a
few challenges:

- The updates are not managed as code if you are manually updating the `ConfigMap`. This can be a problem when you want
  to spin up a new cluster with the same configuration, as you now have to download the `ConfigMap` and replicate it
  into the new cluster.

- The [eks-k8s-role-mapping module](../eks-k8s-role-mapping) allows you to manage the central `ConfigMap` as code.
  However, EKS will create the `ConfigMap` under certain conditions (e.g. to allow access to Fargate), and depending on
  timing, you can end up with an error where terraform is not able to create the `ConfigMap` until you import it into
  the state.

- A single typo or mistake can disable the entire `ConfigMap`. For example, if you have a syntactic yaml error in the
  central `ConfigMap`, it will prevent EKS from being able to read the `ConfigMap`, thereby disabling access to all
  the users captured in the `ConfigMap`.

The `aws-auth-merger` can be used to address these challenges by breaking up the central `ConfigMap` across multiple
`ConfigMaps` that are tracked in a separate place. The `aws-auth-merger` watches for `aws-auth` compatible `ConfigMaps`
that can be merged to manage the `aws-auth` authentication `ConfigMap` for EKS.

The `aws-auth-merger` works as follows:

- When starting up, the `aws-auth-merger` will scan if the main `aws-auth` `ConfigMap` already exists in the
  `kube-system` namespace. The `aws-auth-merger` checks if the `ConfigMap` was created by the merger, and if not, will
  snapshot the `ConfigMap` so that it will be included in the merge.
- The `aws-auth-merger` then does an initial merger of all the `ConfigMaps` in the configured namespace to create the
  initial version of the main `aws-auth` `ConfigMap`.
- The `aws-auth-merger` then enters an infinite event loop that watches for changes to the `ConfigMaps` in the
  configured namespace. The syncing routine will run every time the merger detects changes in the namespace.

## How do I use the aws-auth-merger?

To deploy the `aws-auth-merger`, follow the following steps:

1. Create a docker repository to house the `aws-auth-merger`. We recommend using ECR.
1. Build a Docker image that runs the `aws-auth-merger` and push the container to ECR.
1. Deploy this module using terraform:
    1. Set the `aws_auth_merger_image` variable to point to the ECR repo and tag for the `aws-auth-merger` docker image.
    1. Set additional variables as needed.

If you wish to manually deploy the `aws-auth-merger` without using Terraform, you can deploy a `Deployment` with a
single replica using the image. The `ServiceAccount` that you associate with the `Pods` in the `Deployment` needs to be
able to:

- `get`, `list`, `create`, and `watch` for `ConfigMaps` in the namespace that it is watching.
- `get`, `create`, and `update` the `aws-auth` `ConfigMap` in the `kube-system`.

Once the `aws-auth-merger` is deployed, you can create `ConfigMaps` in the watched namespace that mimic the `aws-auth`
`ConfigMap`. Refer to [the official AWS docs](https://docs.aws.amazon.com/eks/latest/userguide/add-user-role.html) for
more information on the format of the `aws-auth` `ConfigMap`.

For convenience, you can use the [eks-k8s-role-mapping](../eks-k8s-role-mapping) module to manage each individual
`aws-auth` `ConfigMap` to be merged by the merger. Refer to the [eks-cluster-with-iam-role-mappings
example](/example/eks-cluster-with-iam-role-mappings) for an example of how to integrate the two modules.

## How do I handle conflicts with automatic updates by EKS?

EKS will automatically update or create the central `aws-auth` `ConfigMap`. This can lead to conflicts with the
`aws-auth-merger`, including potential data loss that locks out Fargate or Managed Node Group workers. To handle these
conflicts, we recommend the following approach:

- If you are using Fargate for the Control Plane components (e.g. CoreDNS) or for the `aws-auth-merger` itself, ensure
  that the relevant Fargate Profiles are created prior to the initial deployment of the `aws-auth-merger`. This ensures
  that AWS constructs the `aws-auth` `ConfigMap` before the `aws-auth-merger` comes online, allowing it to snapshot the
  existing `ConfigMap` to be merged in to the managed central `ConfigMap`.

- If you are using Fargate outside of the `aws-auth-merger`, ensure that you create the Fargate Profile after the
  `aws-auth-merger` is deployed. Then, create an `aws-auth` `ConfigMap` in the merger namespace that includes the
  Fargate execution role (the input variable `eks_fargate_profile_executor_iam_role_arns` in the
  `eks-k8s-role-mapping` module). This ensures that the Fargate execution role is included in the merged `ConfigMap`.

- If you are using Managed Node Groups, you have two options:
    - Ensure that the Managed Node Group is created prior to the `aws-auth-merger` being deployed. This ensures that AWS
      constructs the `aws-auth` `ConfigMap` before the `aws-auth-merger` comes online.
    - If you wish to create Managed Node Groups after the `aws-auth-merger` is deployed, ensure that the worker IAM role
      of the Managed Node Group is included in an `aws-auth` `ConfigMap` in the merger namespace (the input variable
      `eks_worker_iam_role_arns`).
