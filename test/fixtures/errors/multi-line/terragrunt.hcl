errors {
  # Retry block for transient errors
  retry "transient_errors" {
    retryable_errors = [
      "(?s).*cannot create resource \"storageclasses\" in API group.*",
    ]
    max_attempts       = 3
    sleep_interval_sec = 20
  }

  ignore "transit_gateway_errors" {
    ignorable_errors = [
      ".*creating Route in Route Table*"
    ]
    message = "Ignoring transit gateway not found when creating internal route."
  }
}