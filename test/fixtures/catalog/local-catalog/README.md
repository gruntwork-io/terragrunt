# Local Catalog

`helpers.LocalGitRemote` serves this directory as a git remote, so the catalog
tests never clone from a git host.

The catalog counts every directory under `modules/` holding at least one `.tf`
file as a module, so this tree holds four. Each is named for what it covers:

| Module | Covers |
| --- | --- |
| `modules/with-variables` | a module with inputs, which the scaffold tests generate a configuration from |
| `modules/without-variables` | a module the catalog lists but has no inputs to scaffold |
| `modules/nested/with-readme` | discovery below the first level, with a README supplying the title |
| `modules/nested/without-readme` | the same, with no README, so the title falls back to the directory name |
