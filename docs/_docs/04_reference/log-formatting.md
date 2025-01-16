---
layout: collection-browser-doc
title: Log Formatting
category: reference
categories_url: reference
excerpt: Learn how customize Terragrunt logging.
tags: ["log"]
order: 409
nav_title: Documentation
nav_title_link: /docs/
redirect_from:
    - /docs/features/custom-log-format/
    - /docs/features/log-formatting/
---

Using the `--terragrunt-log-custom-format <format>` flag you can customize the way Terragrunt logs with total control over the logging format.

The argument passed to this flag is a Terragrunt native format string that has special syntax, as described below.

## Placeholders

The format string consists of placeholders and text. Placeholders start with the `%` sign.

e.g.

```shell
--terragrunt-log-custom-format "%time %level %msg"
```

Output:

```shell
10:09:19.809 debug Running command: tofu --version
```

To escape the `%` character, use `%%`.

e.g.

```shell
--terragrunt-log-custom-format "%time %level %%msg"
```

Output:

```shell
10:09:19.809 debug %msg
```

Placeholders have preset names:

* `%time` - Current time.

* `%interval` - Seconds elapsed since Terragrunt started.

* `%level` - Log level.

* `%prefix` - Path to the working directory were Terragrunt is running.

* `%msg` - Log message.

* `%tf-path` - Path to the OpenTofu/Terraform executable (as defined by [terragrunt-tfpath](https://terragrunt.gruntwork.io/docs/reference/cli-options/#terragrunt-tfpath)).

* `%tf-command` - Executed OpenTofu/Terraform command, e.g. `apply`.

* `%tf-command-args` - Arguments of the executed OpenTofu/Terraform command, e.g. `apply -auto-approve`.

* `%t` - Indent.

* `%n` - Newline.

Any other text is considered plain text. The parser always tries to find the longest name. For example, tofu command "apply -auto-approve" with format "%tf-command-args" will be replaced with "apply -auto-approve", but not "apply-args". If you need to replace it with "apply-args", use empty brackets "%tf-command()-args". More examples: "%tf-path" will be replaced with "tofu", `%t()-path` will be replaced with "   -path".

e.g.

```shell
--terragrunt-log-custom-format "time=%time level=%level message=%msg"
```

Output:

```shell
time=00:10:44.716 level=debug message=Running command: tofu --version
```

Using the placeholder as shown above will display the value simply. If you would like to format the value, you can pass options to the placeholder.

Placeholder formatting uses the following syntax:

`%placeholder-name(option-name=option-value, option-name=option-value,...)`

e.g.

```shell
--terragrunt-log-custom-format "%time(format='Y-m-d H:i:sv') %level(format=short,case=upper) %msg"
```

Output:

```shell
2024-11-12 11:52:20.214 DEB Running command: tofu --version
```

In this example, the timestamp (as referenced by the `%time` placeholder) has been formatted with the `format` string `Y-m-d H:i:sv`. Similarly, the log level (as referenced by the `%level` placeholder), has been formatted to use the `short` `format`, and `upper` `case`.

Even if you don't pass options, the empty parenthesis are added implicitly. Thus `%time` is equivalent to `%time()`. Parenthesis are considered part of the syntax for specifying  parameters to placeholders by default. Any parenthesis following a placeholder will be interpreted as specifying the parameters for the placeholder function.

e.g.

```shell
--terragrunt-log-custom-format "%time(plain-text)"
```

Output:

```shell
invalid option name "plain-text" for placeholder "time"
```

If you would like to escape parentheses so that they appear as plain text in logs, make sure to use empty parentheses after a placeholder so that the following parentheses are not evaluated as specifying parameters for the placeholder function.

e.g.

```shell
--terragrunt-log-custom-format "%time()(plain-text)"
```

Output:

```shell
12:33:08.513(plain-text)
```

You can format plain text as well by using an unnamed placeholder.

e.g.

```shell
--terragrunt-log-custom-format "%(content='time=',color=magenta)%time %(content='level=',color=light-blue)%level %(content='msg=',color=green)%msg"
```

Output:

```shell
time=12:33:08.513 level=debug msg=Running command: tofu --version
```

*Unfortunately, it is not possible to display color in a Markdown document, but in the above output, `time=` is colored magenta, `level=` is colored light blue and `msg=` is colored green.*

[![screenshot](/assets/img/screenshots/custom-log-format-1.jpg){: width="50%" }](https://terragrunt.gruntwork.io/assets/img/screenshots/custom-log-format-1.jpg)

## Options

Options can be divided into common ones, which can be passed to any placeholder, and specific ones for each placeholder.

Common options:

* `content=<text>` - Sets a placeholder value, typically used to set the initial value of an unnamed placeholder.

* `case=[upper|lower|capitalize]` - Sets the case of the text.

* `width=<number>` - Sets the column width.

* `align=[left|center|right]` - Aligns content relative to the edges of the column, used in conjunction with `width`.

* `prefix=<text>` - Prepends the prefix to the content. If the content of the placeholder is empty, the prefix will not be prepended.

* `suffix=<text>`-  Appends the suffix to the content. If the content of the placeholder is empty, the suffix will not be appended.

* `escape=[json]` - Escapes content for use as a value in a JSON string.

* `color=[red|white|yellow|green|cayn|magenta|blue|...]` - Sets the color for the content.

  * `1..255` - Specifies a color using a [number](https://www.hackitu.de/termcolor256/), 1 to 255

  * `red|white|yellow|green|cyan|magenta|blue|light-blue|light-black|light-red|light-green|light-yellow|light-magenta|light-cyan|light-white` - Specifies a color using a word

  * `gradient` - Specifies to use a different color each time the placeholder content changes.

  * `preset` - Specifies to use preset colors. For example, each log level name has its own preset color.

  * `disable` - Disables color, also removes colors set in terraform/tofu output.

Specific options for placeholders:

* `%level`

  * `format=[full|short|tiny]` - Specifies the format for log level names.

    * `full` - `stdout`, `stderr`, `error`, `warn`, `info`, `debug`, `trace`

    * `short` - `std`, `err`, `wrn`, `inf`, `deb`, `trc`

    * `tiny` - `s`, `e`, `w`, `i`, `d`, `t`

* `%time`

  * `format=<time-format>` - Sets the time format.

    Preset formats:

    * `date-time` - e.g. `2006-01-02 15:04:05`

    * `date-only` - e.g. `2006-01-02`

    * `time-only` - e.g. `15:04:05`

    * `rfc3339` - e.g. `2006-01-02T15:04:05Z07:00`

    * `rfc3339-nano` - e.g. `2006-01-02T15:04:05.999999999Z07:00`

    Custom format string characters:

    * `H` - 24-hour format of an hour with leading zeros, `00` to `23`

    * `h` - 12-hour format of an hour with leading zeros, `01` to `12`

    * `g` - 12-hour format of an hour without leading zeros, `1` to `12`

    * `i` - Minutes with leading zeros, `00` to `59`

    * `s` - Seconds with leading zeros, `00` to `59`

    * `v` - Milliseconds. e.g. `.654`

    * `u` - Microseconds, e.g. `.654321`

    * `Y` - A full numeric representation of a year, e.g. `1999`, `2003`

    * `y` - A two digit representation of a year, e.g. `99`, `03`

    * `m` - Numeric representation of a month, with leading zeros, `01` to `12`

    * `n` - Numeric representation of a month, without leading zeros, `1` to `12`

    * `M` - A short textual representation of a month, three letters, `Jan` to `Dec`

    * `d` - Day of the month, 2 digits with leading zeros, `01` to `31`

    * `j` - Day of the month without leading zeros, `1` to `31`

    * `D` - A textual representation of a day, three letters, `Mon` to `Sun`

    * `A` - Uppercase Ante meridiem and Post meridiem, `AM` or `PM`

    * `a` - Lowercase Ante meridiem and Post meridiem, `am` or `pm`

    * `T` - Timezone abbreviation, e.g. `EST`, `MDT`

    * `P` - Difference to Greenwich time (GMT) with colon between hours and minutes, e.g. `+02:00`

    * `O` - Difference to Greenwich time (GMT) without colon between hours and minutes, e.g. `+0200`

* `%prefix`

  * `path=[relative|short-relative|short]`

    * `relative` - Outputs a relative path to the working directory.

    * `short-relative` - Outputs a relative path to the working directory, trims the leading slash `./` and hides the working directory path `.`

    * `short` - Outputs an absolute path, but hides the working directory path.

* `%tf-path`

  * `path=[filename|dir]`

    * `filename` - Outputs the name of the executable.

    * `dir` - Outputs the directory name of the executable.

* `%msg`

  * `path=[relative]`

    * `relative` - Converts all absolute paths to paths relative to the working directory.

## Presets

The examples below replicate the preset formats specified with `--terragrunt-log-format`. They can be useful if you need to change existing formats to suit your needs.

### Pretty

`--terragrunt-log-format pretty`

```shell
--terragrunt-log-custom-format "%time(color=light-black) %level(case=upper,width=6,color=preset) %prefix(path=short-relative,color=gradient,suffix=' ')%tf-path(color=cyan,suffix=': ')%msg(path=relative)"
```

### Bare

`--terragrunt-log-format bare`

```shell
--terragrunt-forward-tf-stdout --terragrunt-log-custom-format "%level(case=upper,width=4)[%interval] %msg %prefix(path=short,prefix='prefix=[',suffix=']')"
```

### Key-value

`--terragrunt-log-format key-value`

```shell
--terragrunt-log-custom-format "time=%time(format=rfc3339) level=%level prefix=%prefix(path=short-relative) tf-path=%tf-path(path=filename) msg=%msg(path=relative,color=disable)"
```

### JSON

`--terragrunt-log-format json`

```shell
--terragrunt-log-custom-format '{"time":"%time(format=rfc3339,escape=json)", "level":"%level(escape=json)", "prefix":"%prefix(path=short-relative,escape=json)", "tf-path":"%tf-path(path=filename,escape=json)", "msg":"%msg(path=relative,escape=json,color=disable)"}'
```
