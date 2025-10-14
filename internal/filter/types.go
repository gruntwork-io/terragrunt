package filter

// Unit represents a minimal Terragrunt unit that can be filtered.
// This is a simplified representation focused only on the fields needed for filtering.
type Unit struct {
	// Name is the name of the unit (typically the directory basename)
	Name string

	// Path is the absolute or relative path to the unit
	Path string
}
