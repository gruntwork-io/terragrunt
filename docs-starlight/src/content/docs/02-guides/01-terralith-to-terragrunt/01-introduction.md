---
title: Introduction
description: Introduction to the Terralith to Terragrunt guide
slug: docs/guides/terralith-to-terragrunt
sidebar:
  order: 1
prev: false
---

A common challenge that emerges as infrastructure grows is the "Terralith," a portmanteau of Terraform and Monolith. This pattern, also referred to as a "Megamodule" or an "All In One State" configuration, describes a scenario where a large, complex infrastructure estate is managed within a single state file.

Imagine you're a platform engineer, and what once felt like an instant `tofu apply` to update your infrastructure now drags on for minutes. You once had confidence that you could reliably update exactly the infrastructure you cared about changing in every `tofu apply`, but things have gotten complicated. You now have to sift through a massive wall of plan text to confirm that your intended tag update on a resource doesn't bring down production. You're seeing irrelevant timestamp updates, changes introduced out-of-band from colleagues in the AWS console, and more.

Maybe it's faster (and safer) to just go ahead and make the update out-of-band yourself instead of dealing with this monstrosity, exacerbating the issue. This is the scenario that platform engineers find themselves in when they're struggling to deal with a Terralith. This isn't a hypothetical scenario, it's daily life for teams that take what once worked perfectly fine at smaller scales, and incrementally added more and more complexity and technical debt until they find themselves in the position where they can no effectively longer use the Infrastructure as Code (IaC) that served them so well in the past.

This is a comprehensive, hands-on guide that will demonstrate how you can naturally find yourself managing a Terralith, and how you can get yourself out, using Terragrunt. Along the way, you'll learn skills like:

- Strategies for effectively organizing your IaC for maximum productivity and safety.
- Principles of state manipulation, and a look under the hood as to how OpenTofu/Terraform store state.
- Modern best practices for authoring scalable and reliable Terragrunt configuration.

The guide will do this by having you:

1. Set up a local development environment for managing IaC.
2. Build a fun toy project, and provision it in a real AWS account.
3. Expand that toy project until you can start to see the impact of managing infrastructure following a Terralith architecture pattern.
4. Break down that Terralith to gain improvements in scalability and reliability.
5. Add Terragrunt to improve your ability to orchestrate your IaC.
6. Leverage more of Terragrunt to further improve your DevEx with IaC.

This guide will not assume a significant amount of technical skill with Terragrunt, OpenTofu, AWS or NodeJS, but you will use these tools along the way. The guide will gently guide you through their usage, focusing on teaching you lessons as they pertain to IaC. In the next step of this guide, we'll make sure you have all of these tools installed (and that you're signed up for an AWS account).

The guide will assume that you're comfortable using a terminal, and that you have access to a Unix-like environment, either by using a Linux/macOS workstation, or by using Windows Subsystem for Linux (WSL) on Windows. It will also assume that you're OK with not worrying about certain technical details like how the NodeJS application that you deploy as part of this guide works, as some of those technical details will be glossed over in the interest of focus on the technical details that are relevant in this guide.

While not a requirement, it would be good to have a basic understanding of how Git works, so that you can commit updates to your copy of the project as you go along.

If you get lost or confused at any point, ask for help in the [Terragrunt Discord](https://discord.gg/YENaT9h8jh)! There are plenty of passionate Terragrunt community members that are more than happy to help.
