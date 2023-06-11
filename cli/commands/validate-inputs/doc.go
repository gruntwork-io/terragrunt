package validateinputs

// `validate-inputs` command collects all the terraform variables defined in the target module, and the terragrunt
// inputs that are configured, and compare the two to determine if there are any unused inputs or undefined required
// inputs.
