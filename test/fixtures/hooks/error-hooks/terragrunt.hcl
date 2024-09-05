terraform {
  source = "."

  error_hook "pattern_matching_hook" {
    commands = ["apply"]
    execute  = ["echo", "pattern_matching_hook"]
    on_errors = [
      "not-existing-file.txt"
    ]
  }

  error_hook "catch_all_matching_hook" {
    commands = ["apply"]
    execute  = ["echo", "catch_all_matching_hook"]
    on_errors = [
      ".*"
    ]
  }

  error_hook "not_matching_hook" {
    commands = ["apply"]
    execute  = ["echo", "not_matching_hook"]
    on_errors = [
      ".*random-not-matching-pattern.*"
    ]
  }

}

