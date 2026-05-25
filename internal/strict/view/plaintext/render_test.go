package plaintext_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/strict/view/plaintext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRender(t *testing.T) {
	t.Parallel()

	assert.NotNil(t, plaintext.NewRender())
}

func TestRenderList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		controls strict.Controls
		contains []string
	}{
		{
			name:     "empty",
			controls: strict.Controls{},
		},
		{
			name: "single control",
			controls: strict.Controls{
				&controls.Control{Name: "a", Description: "desc a"},
			},
			contains: []string{"a", "desc a"},
		},
		{
			name: "multiple controls",
			controls: strict.Controls{
				&controls.Control{Name: "a", Description: "desc a"},
				&controls.Control{Name: "b", Description: "desc b"},
			},
			contains: []string{"a", "desc a", "b", "desc b"},
		},
		{
			name: "control falls back to warning when description is empty",
			controls: strict.Controls{
				&controls.Control{Name: "x", Warning: "warn x"},
			},
			contains: []string{"x", "warn x"},
		},
	}

	render := plaintext.NewRender()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := render.List(tc.controls)
			require.NoError(t, err)

			for _, fragment := range tc.contains {
				assert.Contains(t, got, fragment)
			}
		})
	}
}

func TestRenderDetailControl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		control  strict.Control
		contains []string
	}{
		{
			name:    "no subcontrols",
			control: &controls.Control{Name: "parent"},
		},
		{
			name: "with subcontrol",
			control: &controls.Control{
				Name: "parent",
				Subcontrols: strict.Controls{
					&controls.Control{Name: "sub-a", Description: "desc a"},
				},
			},
			contains: []string{"sub-a", "desc a"},
		},
		{
			name: "duplicate subcontrols deduplicated",
			control: &controls.Control{
				Name: "parent",
				Subcontrols: strict.Controls{
					&controls.Control{Name: "dup", Description: "first"},
					&controls.Control{Name: "dup", Description: "second"},
				},
			},
			contains: []string{"dup", "first"},
		},
		{
			name: "subcontrol falls back to warning",
			control: &controls.Control{
				Name: "parent",
				Subcontrols: strict.Controls{
					&controls.Control{Name: "sub-x", Warning: "warn x"},
				},
			},
			contains: []string{"sub-x", "warn x"},
		},
	}

	render := plaintext.NewRender()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := render.DetailControl(tc.control)
			require.NoError(t, err)

			for _, fragment := range tc.contains {
				assert.Contains(t, got, fragment)
			}
		})
	}
}
