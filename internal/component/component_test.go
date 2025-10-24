package component_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentsSort(t *testing.T) {
	t.Parallel()

	// Setup
	configs := component.Components{
		{Path: "c", Kind: component.Unit},
		{Path: "a", Kind: component.Unit},
		{Path: "b", Kind: component.Stack},
	}

	// Act
	sorted := configs.Sort()

	// Assert
	require.Len(t, sorted, 3)
	assert.Equal(t, "a", sorted[0].Path)
	assert.Equal(t, "b", sorted[1].Path)
	assert.Equal(t, "c", sorted[2].Path)
}

func TestComponentsFilter(t *testing.T) {
	t.Parallel()

	// Setup
	configs := component.Components{
		{Path: "unit1", Kind: component.Unit},
		{Path: "stack1", Kind: component.Stack},
		{Path: "unit2", Kind: component.Unit},
	}

	// Test unit filtering
	t.Run("filter units", func(t *testing.T) {
		t.Parallel()

		units := configs.Filter(component.Unit)
		require.Len(t, units, 2)
		assert.Equal(t, component.Unit, units[0].Kind)
		assert.Equal(t, component.Unit, units[1].Kind)
		assert.ElementsMatch(t, []string{"unit1", "unit2"}, units.Paths())
	})

	// Test stack filtering
	t.Run("filter stacks", func(t *testing.T) {
		t.Parallel()

		stacks := configs.Filter(component.Stack)
		require.Len(t, stacks, 1)
		assert.Equal(t, component.Stack, stacks[0].Kind)
		assert.Equal(t, "stack1", stacks[0].Path)
	})
}

func TestComponentsCycleCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupFunc     func() component.Components
		name          string
		errorExpected bool
	}{
		{
			name: "no cycles",
			setupFunc: func() component.Components {
				a := &component.Component{Path: "a"}
				b := &component.Component{Path: "b"}
				a.AddDependency(b)
				return component.Components{a, b}
			},
			errorExpected: false,
		},
		{
			name: "direct cycle",
			setupFunc: func() component.Components {
				a := &component.Component{Path: "a"}
				b := &component.Component{Path: "b"}
				a.AddDependency(b)
				b.AddDependency(a)
				return component.Components{a, b}
			},
			errorExpected: true,
		},
		{
			name: "indirect cycle",
			setupFunc: func() component.Components {
				a := &component.Component{Path: "a"}
				b := &component.Component{Path: "b"}
				c := &component.Component{Path: "c"}
				a.AddDependency(b)
				b.AddDependency(c)
				c.AddDependency(a)
				return component.Components{a, b, c}
			},
			errorExpected: true,
		},
		{
			name: "diamond dependency - no cycle",
			setupFunc: func() component.Components {
				a := &component.Component{Path: "a"}
				b := &component.Component{Path: "b"}
				c := &component.Component{Path: "c"}
				d := &component.Component{Path: "d"}
				a.AddDependency(b)
				a.AddDependency(c)
				b.AddDependency(d)
				c.AddDependency(d)
				return component.Components{a, b, c, d}
			},
			errorExpected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			configs := tt.setupFunc()

			cfg, err := configs.CycleCheck()
			if tt.errorExpected {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cycle detected")
				assert.NotNil(t, cfg)
			} else {
				require.NoError(t, err)
				assert.Nil(t, cfg)
			}
		})
	}
}
