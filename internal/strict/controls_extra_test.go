package strict_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestControlsSuppressWarning(t *testing.T) {
	t.Parallel()

	bootErr := errors.New("active error")
	ctrl := &controls.Control{
		Name:    "a",
		Status:  strict.ActiveStatus,
		Error:   bootErr,
		Warning: "warn a",
		Enabled: true,
	}

	ctrls := strict.Controls{ctrl}
	returned := ctrls.SuppressWarning()
	assert.Equal(t, ctrls, returned)

	err := ctrls.Evaluate(t.Context())
	require.ErrorIs(t, err, bootErr)
}

func TestControlsRemoveDuplicates(t *testing.T) {
	t.Parallel()

	a1 := &controls.Control{Name: "a"}
	a1Dup := &controls.Control{Name: "a"}
	b := &controls.Control{Name: "b"}

	got := strict.Controls{a1, a1Dup, b}.RemoveDuplicates()
	assert.Equal(t, strict.Controls{a1, b}, got)
}

func TestControlsAddSubcontrols(t *testing.T) {
	t.Parallel()

	parentA := &controls.Control{Name: "a"}
	parentB := &controls.Control{Name: "b"}
	sub := &controls.Control{Name: "sub"}

	strict.Controls{parentA, parentB}.AddSubcontrols(sub)

	assert.Equal(t, strict.Controls{sub}, parentA.GetSubcontrols())
	assert.Equal(t, strict.Controls{sub}, parentB.GetSubcontrols())
}

func TestControlsGetSubcontrols(t *testing.T) {
	t.Parallel()

	subA := &controls.Control{Name: "sub-a"}
	subB := &controls.Control{Name: "sub-b"}

	parents := strict.Controls{
		&controls.Control{Name: "p1", Subcontrols: strict.Controls{subA}},
		&controls.Control{Name: "p2", Subcontrols: strict.Controls{subB}},
		&controls.Control{Name: "p3"},
	}

	got := parents.GetSubcontrols()
	assert.Equal(t, strict.Controls{subA, subB}, got)
}

func TestControlsSort(t *testing.T) {
	t.Parallel()

	a := &controls.Control{Name: "a", Status: strict.ActiveStatus}
	b := &controls.Control{Name: "b", Status: strict.ActiveStatus}
	c := &controls.Control{Name: "c", Status: strict.CompletedStatus}
	empty := &controls.Control{Name: ""}

	tests := []struct {
		name  string
		input strict.Controls
		want  strict.Controls
	}{
		{
			name:  "different statuses sort by status",
			input: strict.Controls{c, a},
			want:  strict.Controls{a, c},
		},
		{
			name:  "equal statuses sort by name",
			input: strict.Controls{b, a},
			want:  strict.Controls{a, b},
		},
		{
			name:  "empty name j short-circuits",
			input: strict.Controls{a, empty},
			want:  strict.Controls{empty, a},
		},
		{
			name:  "empty name i short-circuits",
			input: strict.Controls{empty, a},
			want:  strict.Controls{empty, a},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sorted := tc.input.Sort()
			assert.Equal(t, tc.want, sorted)
			assert.Len(t, sorted, len(tc.input))
		})
	}
}

func TestControlsEvaluate(t *testing.T) {
	t.Parallel()

	bootErr := errors.New("boom")

	t.Run("empty slice returns nil", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, strict.Controls{}.Evaluate(context.Background()))
	})

	t.Run("returns first error encountered", func(t *testing.T) {
		t.Parallel()

		good := &controls.Control{Name: "good"}
		bad := &controls.Control{
			Name:    "bad",
			Status:  strict.ActiveStatus,
			Error:   bootErr,
			Enabled: true,
		}

		err := strict.Controls{good, bad}.Evaluate(context.Background())
		require.ErrorIs(t, err, bootErr)
	})
}
