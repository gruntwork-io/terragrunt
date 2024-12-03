feature "feature_name" {
  default = false
}

errors {
  # Retry configuration block that allows for retrying errors that are known to be intermittent
  # Note that this replaces `retryable_errors`, `retry_max_attempts` and `retry_sleep_interval_sec` fields.
  # Those fields will still be supported for backwards compatibility, but this block will take precedence.
  retry "foo" {
    retryable_errors = !feature.feature_name.value ? [] : [
      ".*Error: foo.*"
    ]
    max_attempts       = 3
    sleep_interval_sec = 5
  }

  # Ignore configuration block that allows for ignoring errors that are known to be safe to ignore
  ignore "bar" {
    # Specify a pattern that will be detected in the error for ignores, or just ignore any error
    ignorable_errors = [
      ".*Error: bar.*", # If STDERR includes "Error: bar", ignore it
      "!.*Error: baz.*" # If STDERR includes "Error: baz", do not ignore it
    ]
    message = "Ignoring error bar" # Add an optional warning message if it fails
    # Key-value map that can be used to emit signals to external systems on failure
    signals = {
      safe_to_revert = true # Signal that the apply is safe to revert on failure
    }

  }
}