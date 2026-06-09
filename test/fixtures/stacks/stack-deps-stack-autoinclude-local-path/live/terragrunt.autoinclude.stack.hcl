# Hand-authored sibling autoinclude: the injected unit's path references the base stack's local.region.
# This mirrors a stale or hand-edited autoinclude whose path expression is not baked to a literal.
unit "extra" {
  source = "${get_repo_root()}/units/extra"
  path   = "extra-${local.region}"
}
