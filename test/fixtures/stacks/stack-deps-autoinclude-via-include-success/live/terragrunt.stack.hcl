// Root has only a literal `include`. The included file declares parser-compatible autoinclude with mock_outputs + inputs. Generation must slice expressions from the included file's bytes, not the root's.

include "shared" {
  path = "./shared.hcl"
}
