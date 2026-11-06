output "files" {
  value = {
    dot_slash      = fileset("./", "./resources/*.json")
    canonical      = fileset("./", "resources/*.json")
    double_star    = fileset(".", "./resources/**/*.json")
    absolute_path  = fileset(abspath(path.module), "./resources/*.json")
    parent         = fileset("./resources", "../resources/*.json")
    above          = fileset(".", "../shared/*.json")
    double_slash   = fileset(".", "resources//*.json")
    dot_segment    = fileset(".", "resources/./*.json")
    parent_segment = fileset(".", "resources/sub/../*.json")
    included       = fileset("..", "./resources/*.json")
  }
}
