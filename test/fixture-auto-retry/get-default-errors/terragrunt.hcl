inputs = {
  retryable_errors = concat(get_default_retryable_errors(), ["my special snowflake"])
}
