# An autoinclude patches a unit with config beyond dependency/inputs. Here a
# generate block is injected; it must land in the generated unit and emit the file
# when the unit runs.
unit "gen" {
  source = "../catalog/units/gen"
  path   = "gen"

  autoinclude {
    generate "injected" {
      path      = "injected.tf"
      if_exists = "overwrite"
      contents  = <<-TF
        output "injected" {
          value = "injected-by-autoinclude"
        }
      TF
    }
  }
}
