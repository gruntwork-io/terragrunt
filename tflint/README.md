# tflint 

This package is a wrapper of the ![tflint](https://github.com/terraform-linters/tflint) project, enabling it to be natively executed from the before and after hooks. The `tflint/tflint.go` uses the MPL 2.0 license, due to the license of tflint.

## How to use

This package allows us to embed [tflint](https://github.com/terraform-linters/tflint) in Terragrunt, enabling it to be natively executed from the before and after hooks without having to install `tflint` separately. Since `tflint` is licensed with MPL, we are required to let you know where you can find its source code: https://github.com/terraform-linters/tflint.
}

Here's an example:
```hcl
terraform {
  before_hook "before_hook" {
    commands     = ["apply", "plan"]
    execute      = ["tflint"]
  }
}
```

The `.tflint.hcl` should exist in the same folder or one of it's parents. If Terragrunt can't find a `.tflint.hcl` file, it won't execute tflint.
```hcl
plugin "aws" {
    enabled = true
    version = "0.21.0"
    source  = "github.com/terraform-linters/tflint-ruleset-aws"
}
```

### Configuration

The `execute` parameter only accepts `tflint`, it will ignore any other parameter. Any desired extra configuration should be added in the `.tflint.hcl` file. It will work with a `.tflint.hcl` file in any parent folder.

## Troubleshooting

### `flag provided but not defined: -act-as-bundled-plugin` error

If you have an `.tflint.hcl` file that is empty, or uses the `terraform` ruleset without version or source constraint, it returns the following error:
```
Failed to initialize plugins; Unrecognized remote plugin message: Incorrect Usage. flag provided but not defined: -act-as-bundled-plugin
```

To fix this, make sure that the configuration for the `terraform` ruleset, in the `.tflint.hcl` file contains a version constraint:
```
plugin "terraform" {
    enabled = true
    version = "0.2.1"
    source  = "github.com/terraform-linters/tflint-ruleset-terraform"
}
```

