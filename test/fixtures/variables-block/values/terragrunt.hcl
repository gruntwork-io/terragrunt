variables {
  kind = get_env("TG_TEST_VARIABLES_KIND")

  inputs = {
    number         = 1.5
    large_number   = 123456789012
    flag           = false
    object         = { name = "app", sizes = [1, 2] }
    keyword_keys   = { "for" = "team", "if" = "x" }
    empty_list     = []
    empty_map      = {}
    template       = "$${var.literal}"
    directive      = "%%{if}"
    multiple_lines = "line1\nline2\n"
    unicode        = "h\u00e9llo \"quoted\""
  }
}
