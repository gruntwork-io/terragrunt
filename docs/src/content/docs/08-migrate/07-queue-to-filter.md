---
title: Migrating from Queue Flags to Filter
description: Learn how to migrate from the legacy queue control flags to the modern --filter flag.
slug: migrate/queue-to-filter
sidebar:
  order: 7
---

This guide explains how to migrate from the legacy queue control flags to the modern `--filter` flag.

The queue control flags are currently aliased to their equivalent `--filter` expressions, but will be deprecated in a future version of Terragrunt. We recommend migrating to `--filter` to take advantage of its more flexible and composable syntax.

## Overview

| Legacy Flag | Status | Filter Equivalent |
|---|---|---|
| `--queue-include-dir=<path>` | Aliased to filter | `--filter='<path>'` |
| `--queue-exclude-dir=<path>` | Aliased to filter | `--filter='!<path>'` |
| `--queue-exclude-external` | Now default behavior | Not needed |
| `--queue-strict-include` | Now default behavior | Not needed |
| `--units-that-include=<path>` | Aliased to filter | `--filter='reading=<path>'` |
| `--queue-include-external` | Current | `--filter='{./**}...'` |

## Migrating `--queue-include-dir`

**Before:**

```bash
terragrunt run --all --queue-include-dir=./networking -- plan
```

**After:**

```bash
terragrunt run --all --filter='./networking' -- plan
```

## Migrating `--queue-exclude-dir`

**Before:**

```bash
terragrunt run --all --queue-exclude-dir=./legacy -- plan
```

**After:**

```bash
terragrunt run --all --filter='!./legacy' -- plan
```

## Migrating `--queue-exclude-external`

This flag is no longer needed. External dependencies are now excluded by default.

**Before:**

```bash
terragrunt run --all --queue-exclude-external -- plan
```

**After:**

```bash
# No flag needed — external dependencies are excluded by default
terragrunt run --all -- plan
```

If you need to _include_ external dependencies, use:

```bash
terragrunt run --all --filter='{./**}...' -- plan
```

## Migrating `--queue-strict-include`

This flag is no longer needed. The behavior it enabled (only including units matching `--queue-include-dir`) is now the default.

**Before:**

```bash
terragrunt run --all --queue-include-dir=./networking --queue-strict-include -- plan
```

**After:**

```bash
terragrunt run --all --filter='./networking' -- plan
```

## Migrating `--units-that-include`

**Before:**

```bash
terragrunt run --all --units-that-include=shared.hcl -- plan
```

**After:**

```bash
terragrunt run --all --filter='reading=shared.hcl' -- plan
```
