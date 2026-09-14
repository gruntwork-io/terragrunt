---
question: "Does Pipelines require a specific directory structure?"
title: "Does Pipelines require a specific directory structure? | Terragrunt"
description: "No fixed layout required. HCL configuration as code maps your repo structure to your environments, with dependencies supported across units and environments."
category: "terragrunt-scale"
order: 7
---

Pipelines is flexible and can be configured to support your repository structure using [HCL Configurations as Code](https://docs.gruntwork.io/2.0/reference/pipelines/configurations-as-code/). You programmatically define the relationship your Infrastructure as Code has to your environments, and Pipelines will operate in those environments accordingly. Pipelines supports dependencies between Terragrunt units, even if those units are in different environments or require different cloud authentication credentials.
