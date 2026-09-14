---
question: "What's the recommended approach for using Drift Detection?"
title: "What's the recommended approach for using Drift Detection? | Terragrunt"
description: "Remediate drift gradually by running Drift Detection per environment one-by-one, then schedule weekly automatic runs to keep infrastructure aligned with IaC."
category: "terragrunt-scale"
order: 14
---

If you are suffering from infrastructure that’s experiencing significant drift from your IaC, first, run drift detection in each environment one-by-one to gradually remediate drift over time. Once you are confident your deployed infrastructure is aligned with the configuration you’ve defined using IaC, schedule automatic runs of Drift Detection once a week to ensure you do not accrue drift.
