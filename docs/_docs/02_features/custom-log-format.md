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

Using this `--terragrunt-log-custom-format <format>` flag you can specify which information you want to output.

### Placeholders

The format string consists of placeholders and text. Placeholders start with the `%` sign. The simplest example:

```shell
--terragrunt-log-custom-format "%time %level %msg"
```

Output:

```shell
10:09:19.809 debug Running command: tofu --version
```

The double sign `%%` displays the percent sign as plain text.

```shell
--terragrunt-log-custom-format "%time %level %%msg"
```

Output:

```shell
10:09:19.809 debug %msg
```

Placeholders have preset names:

* `%time` - Current time.

* `%interval` - Seconds has passed since Terragrunt started.

* `%level` - Log level.

* `%prefix` - Path to working directory.

* `%tfpath` - Path to TF executable file.

* `%msg` - Log message.

Any other text is considered as plain text, for example:

```shell
--terragrunt-log-custom-format "time=%time level=%level message=%msg"
```

Output:

```shell
time=00:10:44.716 level=debug message=Running command: tofu --version
```

A placeholder is just a value, to format this value you need to pass options to the placeholder. It has the following syntax:

`%placeholder-name(option-name=option-value, option-name=option-value,...)`

```shell
--terragrunt-log-custom-format "%time(format='Y-m-d H:i:sv') %level(format=short,case=upper) %msg"
```

Output:

```shell
2024-11-12 11:52:20.214 DEB Running command: tofu --version
```

Even if you don't pass options, the empty brackets are added implicitly. Thus `%time` equals `%time()`. If you need to add brackets as plain text after a placeholder with no options and without space, you need to explicitly specify empty brackets first, otherwise, they will be treated as invalid options.

```shell
--terragrunt-log-custom-format "%level()(%time()(%msg))"
```

Output:

```shell
debug(12:15:48.355(Running command: tofu --version))
```

You can format plain text as well by using an unnamed placeholder.

```shell
--terragrunt-log-custom-format "%(content='time=',color=magenta)%time %(content='level=',color=light-blue)%level %(content='msg=',color=green)%msg"
```

Output:

```shell
time=12:33:08.513 level=debug msg=Running command: tofu --version
```

*Unfortunately, it is not possible to display color in a Markdown document, but in the above output, `time=` is colored magenta, `level=` is colored light blue and `msg=` is colored green.*

### Options

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

  * `gradient` - Specifies to use a new color each time the placeholder contents change.

  * `preset` - Specifies to use preset colors. For example, each log level name has its own preset color.

  * `disable` - Disables color, also removes colors set in terraform/tofu output.

Specific options for placeholders:

* `%level`

  * `format=[tiny|short]` - Shortens the log level names `stdout`, `stderr`, `error`, `warn`, `info`, `debug`, `trcace`  to 1 and 3 characters.

    * `tiny` - `std`, `err`, `wrn`, `inf`, `deb`, `trc`

    * `short` - `s`, `e`, `w`, `i`, `d`, `t`

* `%time`

  * `format=<time-format>` - Sets the time format.

    Persets formats:

    * `date-time` - Example: 2006-01-02 15:04:05

    * `date-only` - Example: 2006-01-02

    * `time-only` - Example: 15:04:05

    * `rfc3339` - Example: 2006-01-02T15:04:05Z07:00

    * `rfc3339-nano` - Example: 2006-01-02T15:04:05.999999999Z07:00

    Characters formats:

    * `H` - 24-hour format of an hour with leading zeros, 00 to 23

    * `h` - 12-hour format of an hour with leading zeros, 01 to 12

    * `g` - 12-hour format of an hour without leading zeros, 1 to 12

    * `i` - Minutes with leading zeros, 00 to 59

    * `s` - Seconds with leading zeros, 00 to 59

    * `v` - Milliseconds, example: .654

    * `u` - Microseconds, example: .654321

    * `Y` - A full numeric representation of a year, examples: 1999, 2003

    * `y` - A two digit representation of a year, examples: 99 or 03

    * `m` - Numeric representation of a month, with leading zeros, 01 to 12

    * `n` - Numeric representation of a month, without leading zeros, 1 to 12

    * `M` - A short textual representation of a month, three letters, Jan to Dec

    * `d` - Day of the month, 2 digits with leading zeros, 01 to 31

    * `j` - Day of the month without leading zeros, 1 to 31

    * `D` - A textual representation of a day, three letters, Mon to Sun

    * `A` - Uppercase Ante meridiem and Post meridiem, AM or PM

    * `a` - Lowercase Ante meridiem and Post meridiem, am or pm

    * `T` - Timezone abbreviation, examples: EST, MDT

    * `P` - Difference to Greenwich time (GMT) with colon between hours and minutes, example: +02:00

    * `O` - Difference to Greenwich time (GMT) without colon between hours and minutes, example: +0200

* `%prefix`

  * `path=[relative|short-relative|short]`

    * `relative` - Outputs a relative path to the working directory.

    * `short-relative` - Outputs a relative path to the working directory, trims the leading slash `./` and hides the working directory path `.`

    * `short` - Outputs a abosolute path, but hides the working directory path.

* `%tfpath`

  * `path=[filename|dir]`

    * `filename` - Outputs the name of the executable file.

    * `dir` - Outputs the directory name of the executable file.

* `%msg`

  * `path=[relative]`

    * `relative` - Converts all absolute paths to relative ones to the working directory.

### Examples

The examples below replicate the formats specified with `--terragrunt-log-format`. They can be useful if you need to change existing formats to suit your needs.

`--terragrunt-log-format pretty`

```shell
--terragrunt-log-custom-format "%time(color=light-black) %level(case=upper,width=6,color=preset) %prefix(path=short-relative,color=gradient,suffix=' ')%tfpath(color=cyan,suffix=': ')%msg(path=relative)"
```

`--terragrunt-log-format bare`

```shell
--terragrunt-log-custom-format "%level(case=upper,width=4)[%interval] %msg %prefix(path=short,prefix='prefix=')"
```

`--terragrunt-log-format key-value`

```shell
--terragrunt-log-custom-format "time=%time(format=rfc3339) level=%level prefix=%prefix(path=short-relative) tfpath=%tfpath(path=filename) msg=%msg(path=relative,color=disable)"
```

`--terragrunt-log-format json`

```shell
--terragrunt-log-custom-format '{"time":"%time(format=rfc3339,escape=json)", "level":"%level(escape=json)", "prefix":"%prefix(path=short-relative,escape=json)", "tfpath":"%tfpath(path=filename,escape=json)", "msg":"%msg(path=relative,escape=json,color=disable)"}'
```
