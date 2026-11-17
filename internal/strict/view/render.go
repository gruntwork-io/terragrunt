package view

import "github.com/gruntwork-io/terragrunt/internal/strict"

// Render formats strict controls for `terragrunt info strict`.
type Render interface {
	// List renders the list of controls.
	List(controls strict.Controls) (string, error)

	// DetailSubcontrols renders the subcontrols of a single control.
	DetailSubcontrols(subcontrols strict.Controls) (string, error)
}
