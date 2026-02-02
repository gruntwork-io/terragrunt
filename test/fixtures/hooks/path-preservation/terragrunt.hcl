terraform {
  before_hook "test_path_hook" {
    commands = ["apply", "plan"]
    execute  = ["sh", "-c", "echo 'export TG_PROVIDER_CACHE_DIR=\"/home/testuser/.terraform.d/plugin-cache\"' >&2 && exit 1"]
  }
}
