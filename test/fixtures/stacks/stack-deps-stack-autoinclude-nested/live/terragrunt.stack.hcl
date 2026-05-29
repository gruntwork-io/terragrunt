# Stack-level autoinclude that injects a nested stack (not just a unit). The injected
# stack must itself be re-discovered and expanded by the level-by-level generator, so its
# units materialize one level deeper than the autoinclude target.
stack "networking" {
  source = "../stacks/networking"
  path   = "networking"

  autoinclude {
    stack "more" {
      source = "${get_repo_root()}/stacks/more"
      path   = "more"
    }
  }
}
