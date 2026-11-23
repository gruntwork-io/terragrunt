inputs = {
  src   = "./${get_env("TG_TEST_AUTO_INIT_MODULE", "a")}"
  other = get_env("TG_TEST_AUTO_INIT_OTHER", "x")
}
