terraform {
  # Invalid source URL that will cause download to fail
  source = "/totallyfakedoesnotexist/notreal"

  # This is the correct way to handle source download errors
  error_hook "error_on_source_download" {
    commands  = ["init-from-module"]
    execute   = ["echo", "ERROR_HOOK_TRIGGERED_ON_INIT_FROM_MODULE"]
    on_errors = [".*"]
  }
}
