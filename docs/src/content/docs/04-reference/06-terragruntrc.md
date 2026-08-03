---
title: The .terragruntrc file
description: Commit your Terragrunt defaults instead of asking everyone to export the same environment variables.
slug: reference/terragruntrc
sidebar:
  order: 6
---

The `.terragruntrc` file supplies default environment variables and CLI flag values from a file that
can live next to the configurations it applies to. It is read before Terragrunt parses its command
line, so it can also set variables that have to be in place before Terragrunt starts working, such as
`TF_CLI_CONFIG_FILE`.

:::caution
This is an experimental feature. Enable it with the [`terragruntrc`](/reference/experiments/active#terragruntrc)
experiment:

```bash
terragrunt run plan --experiment terragruntrc
```

```bash
TG_EXPERIMENT='terragruntrc' terragrunt run plan
```

When the experiment is not enabled, a `.terragruntrc` file that is found is reported in a warning and
otherwise ignored.
:::

## Format

The file can be written as JSON or YAML. Terragrunt looks for these names, in this order, within a
single directory:

1. `.terragruntrc.json`
2. `.terragruntrc.yaml`
3. `.terragruntrc.yml`

It has three optional sections:

```json
{
  "env": {
    "TG_PROVIDER_CACHE": "1",
    "TF_CLI_CONFIG_FILE": "./terraformrc.hcl"
  },
  "flags": [
    { "name": "non-interactive", "default": true },
    { "name": "log-level", "default": "debug" }
  ],
  "commands": [
    {
      "name": "run",
      "flags": [{ "name": "parallelism", "default": 8 }]
    }
  ]
}
```

```yaml
env:
  TG_PROVIDER_CACHE: "1"
  TF_CLI_CONFIG_FILE: ./terraformrc.hcl
flags:
  - name: non-interactive
    default: true
  - name: log-level
    default: debug
commands:
  - name: run
    flags:
      - name: parallelism
        default: 8
```

Unknown keys are an error, so that a typo in a file the whole team shares is reported instead of
silently doing nothing.

### `env`

Environment variables to export before Terragrunt reads its environment. Use this for variables that
are not Terragrunt flags, such as `TF_CLI_CONFIG_FILE` or `TERM`.

Values are processed as follows:

- `$VAR` and `${VAR}` are expanded from variables that are already set.
- A value that starts with `./` or `../` is resolved against the directory holding the
  `.terragruntrc` file, so a committed configuration works no matter where the repository is cloned.
- A value containing `://` is left alone, because it is a URL rather than a path.

### `flags`

Defaults for flags that apply to every command. The `name` is the flag name as it is typed on the
command line, without the leading dashes.

The `default` can be a string, a boolean or a number. For a flag that accepts more than one value,
use a list, which is applied as though the flag had been repeated:

```json
{ "flags": [{ "name": "experiment", "default": ["stacks", "terragruntrc"] }] }
```

### `commands`

Defaults that apply only while a single command runs. A command is addressed either by its own name
(`format`) or by its full path as it is typed (`hcl format`). Aliases work too, so `hcl fmt` and `fmt`
address the same command.

```json
{
  "commands": [
    { "name": "run", "flags": [{ "name": "parallelism", "default": 8 }] },
    { "name": "hcl fmt", "flags": [{ "name": "diff", "default": true }] }
  ]
}
```

## Discovery

Terragrunt uses the first file it finds, searching in this order:

1. The working directory, then each parent directory, up to and including the root of the git
   repository.
2. The `.config` directory at the root of the git repository.
3. The `terragrunt` directory inside the user configuration directory (`$XDG_CONFIG_HOME/terragrunt`,
   or `~/.config/terragrunt`).
4. The user home directory.

Files are not merged: the first one found is the only one used. When the working directory is not
inside a git repository, only the working directory itself is searched, so an unrelated ancestor
directory cannot configure a run.

The search starts at [`--working-dir`](/reference/cli/global-flags#working-dir) when it is set, and
at the current directory otherwise.

## Precedence

From highest to lowest:

1. The command line.
2. Environment variables.
3. The `.terragruntrc` file.
4. Terragrunt's built-in defaults.

A variable that is already exported in the shell is never overwritten by the `env` section, and a
flag given on the command line is never overwritten by the `flags` or `commands` sections. That makes
the file safe to commit: it changes the defaults for everyone, and anyone can still override them for
a single run.

## Example

A repository that publishes providers through a private registry, and wants the provider cache server
to see the matching CLI configuration, can commit this next to its root configuration:

```json title=".terragruntrc.json"
{
  "env": {
    "TG_PROVIDER_CACHE": "1",
    "TF_CLI_CONFIG_FILE": "./terraformrc.hcl"
  },
  "flags": [{ "name": "non-interactive", "default": true }]
}
```

Every engineer who clones the repository gets the same provider cache configuration, with no exports
in their shell profile, and CI needs no extra setup step.
