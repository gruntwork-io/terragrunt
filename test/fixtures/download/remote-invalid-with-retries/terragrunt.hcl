terraform {
  source = "https://github.com/totallyfakedoesnotexist/notreal.git//foo?ref=v1.2.3"

}

 errors {
   retry "any" {
     retryable_errors = [".*"]
     max_attempts = 2
     sleep_interval_sec = 1
   }
}

