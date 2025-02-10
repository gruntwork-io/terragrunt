package view

import "github.com/gruntwork-io/terragrunt/internal/strict"

type Render interface {
	// List renders the list of controls.
	List(controls strict.Controls) (string, error)

	// DetailControl renders the detailed information about the control, including its subcontrols.
	DetailControl(control strict.Control) (string, error)
}
