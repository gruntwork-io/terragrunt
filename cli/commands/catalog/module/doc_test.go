package module_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/stretchr/testify/assert"
)

var testFrontmatterEcsCluster = `
<!-- Frontmatter
type: service
name: Amazon ECS Cluster
description: Deploy an Amazon ECS Cluster.
category: docker-orchestration
cloud: aws
tags: ["docker", "orchestration", "ecs", "containers"]
license: gruntwork
built-with: terraform, bash, python, go
-->
# Amazon ECS Cluster

[![Maintained by Gruntwork](https://img.shields.io/badge/maintained%20by-gruntwork.io-%235849a6.svg)](https://gruntwork.io)
`

var testFrontmatterAsgService = `


  <!-- Frontmatter
description: Deploy an AMI across an Auto Scaling Group (ASG), with support for zero-downtime, rolling deployment, load balancing, health checks, service discovery, and auto scaling.
type: service
name: Auto Scaling Group (ASG)
category: services
cloud: aws
tags: ["asg", "ec2"]
license: gruntwork
built-with: terraform
-->

# Auto Scaling Group

[![Maintained by Gruntwork](https://img.shields.io/badge/maintained%20by-gruntwork.io-%235849a6.svg)](https://gruntwork.io)
`

func TestFrontmatter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		content      string
		expectedName string
		expectedDesc string
	}{
		{
			testFrontmatterEcsCluster,
			"Amazon ECS Cluster",
			"Deploy an Amazon ECS Cluster.",
		},
		{
			testFrontmatterAsgService,
			"Auto Scaling Group (ASG)",
			"Deploy an AMI across an Auto Scaling Group (ASG), with support for zero-downtime, rolling deployment, load balancing, health checks, service discovery, and auto scaling.",
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			doc := module.NewDoc(testCase.content, "")

			assert.Equal(t, testCase.expectedName, doc.Title(), "Frontmatter Name")
			assert.Equal(t, testCase.expectedDesc, doc.Description(0), "Frontmatter Description")
		})
	}

}

var testH1EksK8sArgocd = `
# EKS K8s GitOps Module
This module deploys [Argo CD](https://argo-cd.readthedocs.io/en/stable/) to an EKS cluster. Argo CD is a declarative GitOps continuous delivery tool for Kubernetes. See the [Argo CD](https://argo-cd.readthedocs.io/en/stable/) for more details. This module supports deploying the Argo CD resources to Fargate in addition to EC2 Worker Nodes.


# Gruntwork GitOps "GruntOps"

GitOps is an operational framework that is built around DevOps best practices for a standardized approach to managing the lifecycle of Kubernetes based deployments. GitOps provides a unified approach to the deployment and management of container workloads, with Git being the single source of truth for the state of the container infrastructure. GitOps is a very developer-centric workflow that works best when adopted by individuals and teams that follow a git based development lifecycle. The core principles of GitOps have been at the center of Gruntwork from the beginning!


## Getting Started
To use this module, you will need to have a running EKS cluster prior to deploying this module. See the [Argo CD Example](/examples/eks-cluster-with-argocd/) for an example of how to deploy this module.
`

var testH1EksCloudwatchAgent = `
# EKS CloudWatch Agent Module

This Terraform Module installs and configures
[Amazon CloudWatch Agent](https://github.com/aws/amazon-cloudwatch-agent/) on an EKS cluster, so that
each node runs the agent to collect more system-level metrics from Amazon EC2 instances and ship them to Amazon CloudWatch.
This extra metric data allows using [CloudWatch Container Insights](https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/ContainerInsights.html)
for a single pane of glass for application, performance, host, control plane, data plane insights.

This module uses the [community helm chart](https://github.com/aws/eks-charts/tree/8b063ec/stable/aws-cloudwatch-metrics),
with a set of best practices inputs.

**This module is for setting up CloudWatch Agent for EKS clusters with worker nodes (self-managed or managed node groups) that
have support for [` + "`DaemonSets`" + `](https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/). CloudWatch Container
Insights is [not supported for EKS Fargate](https://github.com/aws/containers-roadmap/issues/920).**


## How does this work?

CloudWatch automatically collects metrics for many resources, such as CPU, memory, disk, and network.
Container Insights also provides diagnostic information, such as container restart failures,
to help you isolate issues and resolve them quickly.
`

var testH1EcsCluster = `
# Amazon ECS Cluster

[![Maintained by Gruntwork](https://img.shields.io/badge/maintained%20by-gruntwork.io-%235849a6.svg)](https://gruntwork.io)
![Terraform version](https://img.shields.io/badge/tf-%3E%3D1.1.0-blue.svg)
[![Docs](https://img.shields.io/badge/docs-docs.gruntwork.io-informational)](https://docs.gruntwork.io/reference/services/app-orchestration/amazon-ecs-cluster)

## Overview

This service contains [Terraform](https://www.terraform.io) code to deploy a production-grade ECS cluster on
[AWS](https://aws.amazon.com) using [Elastic Container Service (ECS)](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/Welcome.html).

This service launches an ECS cluster on top of an Auto Scaling Group that you manage. If you wish to launch an ECS
cluster on top of Fargate that is completely managed by AWS, refer to the
[ecs-fargate-cluster module](../ecs-fargate-cluster). Refer to the section
[EC2 vs Fargate Launch Types](https://github.com/gruntwork-io/terraform-aws-ecs/blob/master/core-concepts.md#ec2-vs-fargate-launch-types)
for more information on the differences between the two flavors.
`

var testH1EksAWSAuthMerger = `
:type: service
:name: EKS AWS Auth Merger
:description: Manage the aws-auth ConfigMap across multiple independent ConfigMaps.

// AsciiDoc TOC settings
:toc:
:toc-placement!:
:toc-title:

// GitHub specific settings. See https://gist.github.com/dcode/0cfbf2699a1fe9b46ff04c41721dda74 for details.
ifdef::env-github[]
:tip-caption: :bulb:
:note-caption: :information_source:
endif::[]

= EKS AWS Auth Merger

image:https://img.shields.io/badge/maintained%20by-gruntwork.io-%235849a6.svg[link="https://gruntwork.io/?ref=repo_aws_eks"]
image:https://img.shields.io/badge/tf-%3E%3D1.1.0-blue[Terraform version]
image:https://img.shields.io/badge/k8s-1.24%20~%201.28-5dbcd2[K8s version]

This module contains a go CLI, docker container, and terraform module for deploying a Kubernetes controller for managing
mappings between AWS IAM roles and users to RBAC groups in Kubernetes. The official way to manage the mapping is to add
values in a single, central ` + "`ConfigMap`" + `. This module allows you to break up the central ` + "`ConfigMap`" + ` across multiple,
separate ` + "`ConfigMaps`" + ` each configuring a subset of the mappings you ultimately want to use, allowing you to update
entries in the ` + "`ConfigMap`" + ` in isolated modules (e.g., when you add a new IAM role in a separate module from the EKS
	cluster). The ` + "`aws-auth-merger`" + ` watches for ` + "`aws-auth`" + ` compatible ` + "`ConfigMaps`" + ` that can be merged to manage the
` + "`aws-auth`" + ` authentication ` + "`ConfigMap`" + ` for EKS.


toc::[]




== Features

* Break up the ` + "`aws-auth`" + ` Kubernetes ` + "`ConfigMap`" + ` across multiple objects.
* Automatically merge new ` + "`ConfigMaps`" + ` as they are added and removed.
* Track automatically generated ` + "`aws-auth`" + ` source ` + "`ConfigMaps`" + ` that are generated by EKS.
`

func TestElement(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		content              string
		fileExt              string
		maxDescriptionLength int
		expectedTitle        string
		expectedDescription  string
	}{
		{
			testH1EksK8sArgocd,
			".md",
			200,
			"EKS K8s GitOps Module",
			"This module deploys Argo CD to an EKS cluster. Argo CD is a declarative GitOps continuous delivery tool for Kubernetes. See the Argo CD for more details.",
		},
		{
			testH1EksCloudwatchAgent,
			".md",
			200,
			"EKS CloudWatch Agent Module",
			"This Terraform Module installs and configures Amazon CloudWatch Agent on an EKS cluster, so that each node runs the agent to collect more system-level metrics from Amazon EC2 instances and ship them to Amazon CloudWatch.",
		},
		{
			testH1EcsCluster,
			".md",
			200,
			"Amazon ECS Cluster",
			"This service contains Terraform code to deploy a production-grade ECS cluster on AWS using Elastic Container Service (ECS).",
		},
		{
			testH1EksAWSAuthMerger,
			".adoc",
			200,
			"EKS AWS Auth Merger",
			"This module contains a go CLI, docker container, and terraform module for deploying a Kubernetes controller for managing mappings between AWS IAM roles and users to RBAC groups in Kubernetes.",
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			doc := module.NewDoc(testCase.content, testCase.fileExt)

			assert.Equal(t, testCase.expectedTitle, doc.Title(), "Title")
			assert.Equal(t, testCase.expectedDescription, doc.Description(testCase.maxDescriptionLength), "Description")
		})
	}

}
