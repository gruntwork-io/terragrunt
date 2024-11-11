---
layout: collection-browser-doc
title: Custom Log Format
category: features
categories_url: features
excerpt: Learn how to use terragrunt provider cache.
tags: ["log"]
order: 280
nav_title: Documentation
nav_title_link: /docs/
---

## Custom Log Format

Using this `--terragrunt-log-custom-format <format>` flag you can specify which information you want to output. The format string consists of placeholders and text. The simplest example:


```shell
--terragrunt-log-custom-format "%time %level %msg"
```

Output:

```shell
10:09:19.809 debug Running command: tofu --version
```

The placeholders have preset names:

* `%time` - current time

* `%interval` - seconds has passed since Terragrunt started

* `%level` - log level

* `%prefix` - path to working directory

* `%tfpath` - path to TF executable file

* `%msg` - log message

Everything else is treated as plain text, for example:


```shell
--terragrunt-log-custom-format "time=%time level=%level message=%msg"
```

Output:

```shell
time=00:10:44.716 level=debug message=Running command: tofu --version
```
