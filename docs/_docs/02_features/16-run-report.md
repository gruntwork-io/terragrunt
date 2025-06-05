---
layout: collection-browser-doc
title: Run Report
category: features
categories_url: features
excerpt: Learn how Terragrunt provides detailed reports of runs, and at-a-glance summaries of them.
tags: ["report"]
order: 216
nav_title: Documentation
nav_title_link: /docs/
slug: run-report
---

This feature is still experimental and subject to change. Do not rely on the schema of data emitted by this feature for production use-cases.

To use the report, you will need to enable the [report](/docs/reference/experiments/#report) experiment.

Terragrunt uses an internal data store to track the results of runs when multiple are done at once. You can view this data, both with a high-level summary that is displayed at the end of each run, and via a detailed report that can be requested on-demand (coming soon).

## Run Summary

By default, when performing a queue-based run (e.g. `terragrunt run --all plan`), Terragrunt will emit some additional information to the console after the run is complete.

```bash
$ terragrunt run --all plan

# Omitted for brevity...

❯❯ Run Summary
   Duration:   62ms
   Units:      3
   Succeeded:  3
```

This output is called the "Run Summary". It provides at-a-glance information about the run that was just performed, including the following (as relevant):

- Duration: The duration of the run.
- Units: The number of units that were run.
- Succeeded: The number of units that succeeded (if any did).
- Failed: The number of units that failed (if any did).
- Excluded: The number of units that were excluded from the run (if any were).
- Early Exits: The number of units that exited early, due to a failure in a dependency (if any did).

### Disabling the summary

You can disable the summary output by using the `--summary-disable` flag.

```bash
terragrunt run --all plan --summary-disable
```

The internal report will still be tracked, and is available for generation if requested.

## Run Report

Optionally, you can also generate a detailed report of the run, which has all the information used to generate the run summary.

This isn't possible yet, but will be in the future.
