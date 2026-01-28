package tips

const (
	// DebuggingDocs is the tip that points users to the debugging documentation.
	DebuggingDocs = "debugging-docs"
)

// NewTips returns a new Tips collection with all available tips.
//
// Never remove any of these tips, as removing them will cause a breaking change for users
// using an invocation of `--no-tip` pointing to a non-existent tip.
//
// e.g. `terragrunt run --no-tip=debugging-docs`
//
// If you want to programmatically document that a tip should no longer be
// used after removing it from the codebase, just set `disabled` to `1` here for that tip.
func NewTips() Tips {
	return Tips{
		{
			Name:    DebuggingDocs,
			Message: "TIP (" + DebuggingDocs + "): For help troubleshooting errors, visit https://terragrunt.gruntwork.io/docs/troubleshooting/debugging",
		},
	}
}
