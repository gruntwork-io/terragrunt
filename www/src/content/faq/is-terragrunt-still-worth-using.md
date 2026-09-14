---
question: "Is Terragrunt still worth using?"
title: "Is Terragrunt still worth using? | Terragrunt"
description: "Is Terragrunt still worth using?"
category: "terragrunt"
order: 8
---

Yes. The question comes up every time [OpenTofu](https://opentofu.org) or Terraform ships a new feature, and the answer has not changed. Terragrunt was started in 2016 to plug gaps in Terraform, and some early Terragrunt features, such as state locking, were later adopted by Terraform itself. As OpenTofu and Terraform matured, Terragrunt's focus shifted to higher level concerns: orchestrating infrastructure as code at scale. The relationship is symbiotic, to the point that Gruntwork works with the OpenTofu core team to adopt the features Terragrunt implements to plug gaps in OpenTofu, so Terragrunt can concentrate on orchestration and safety at scale.

That orchestration is why teams keep using it. Terragrunt isolates infrastructure into [units](https://docs.terragrunt.com/features/units/) with their own state, so the blast radius of a change stays limited to the unit being updated instead of putting all of your infrastructure at risk, and it combines units into [stacks](https://docs.terragrunt.com/features/stacks/) that run in dependency order. It also handles the messy, real-world scenarios that OpenTofu/Terraform keep out of scope by design: retrying transient errors, ignoring errors that are safe to ignore, and hooks that codify operational practices, such as backing up a database before every update, so they happen the same way locally and in CI. For the full argument, read [Terragrunt & OpenTofu: Better together](https://www.gruntwork.io/blog/terragrunt-opentofu-better-together).
