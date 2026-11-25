unit "app" {
  source = "${dirname(path.include)}/units/app"
  path   = "${basename(dirname(path.include))}-app"

  values = {
    include   = path.include
    stack_dir = path.stack
  }

  autoinclude {
    inputs = {
      auto_include = path.include
      auto_index   = path[lower("INCLUDE")]
      auto_guard   = "%{if path[lower("INCLUDE")] == path.include}${basename(path.include)}%{else}other%{endif}"
      auto_label   = "${basename(path.include)}:${basename(path.stack)}"
    }
  }
}
