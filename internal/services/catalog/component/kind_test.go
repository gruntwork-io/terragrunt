package component_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

func TestInspect(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		files        []string
		wantKind     component.Kind
		wantCopyKind component.Kind
		want         component.Markers
		isComponent  bool
		copiedByName bool
	}{
		{
			name:        "module",
			files:       []string{"main.tf", "variables.tf"},
			want:        component.Markers{TF: true},
			wantKind:    component.KindModule,
			isComponent: true,
		},
		{
			name:         "unit",
			files:        []string{"terragrunt.hcl"},
			want:         component.Markers{Unit: true},
			wantKind:     component.KindUnit,
			wantCopyKind: component.KindUnit,
			copiedByName: true,
			isComponent:  true,
		},
		{
			name:         "stack",
			files:        []string{"terragrunt.stack.hcl", "policy.json"},
			want:         component.Markers{Stack: true},
			wantKind:     component.KindStack,
			wantCopyKind: component.KindStack,
			copiedByName: true,
			isComponent:  true,
		},
		{
			name:        "template",
			files:       []string{"boilerplate.yml"},
			want:        component.Markers{Template: true},
			wantKind:    component.KindTemplate,
			isComponent: true,
		},
		{
			// The catalog presents this as the unit its author wrote, while
			// `scaffold` reads the same markers and generates from the module.
			name:        "unit beside a module",
			files:       []string{"terragrunt.hcl", "main.tf"},
			want:        component.Markers{Unit: true, TF: true},
			wantKind:    component.KindUnit,
			isComponent: true,
		},
		{
			name:        "template wins over a unit",
			files:       []string{"boilerplate.yml", "terragrunt.hcl"},
			want:        component.Markers{Template: true, Unit: true},
			wantKind:    component.KindTemplate,
			isComponent: true,
		},
		{
			name:         "stack wins over a unit",
			files:        []string{"terragrunt.stack.hcl", "terragrunt.hcl"},
			want:         component.Markers{Stack: true, Unit: true},
			wantKind:     component.KindStack,
			wantCopyKind: component.KindStack,
			copiedByName: true,
			isComponent:  true,
		},
		{
			name:  "the registry placeholder is not a module",
			files: []string{"terraform-cloud-enterprise-private-module-registry-placeholder.tf"},
			want:  component.Markers{},
		},
		{
			name:  "nothing at all",
			files: []string{"README.md"},
			want:  component.Markers{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewMemMapFS()
			dir := "/component"

			for _, name := range tc.files {
				writeFileFS(t, fsys, filepath.Join(dir, name), "")
			}

			markers, err := component.Inspect(fsys, dir)
			require.NoError(t, err)
			assert.Equal(t, tc.want, markers)

			copyKind, copiedByName := markers.CopyKind()
			assert.Equal(t, tc.copiedByName, copiedByName, "scaffold copies these files")
			assert.Equal(t, tc.wantCopyKind, copyKind)

			kind, isComponent := markers.Kind()
			assert.Equal(t, tc.isComponent, isComponent, "the catalog offers this directory")

			if !isComponent {
				return
			}

			assert.Equal(t, tc.wantKind, kind)
		})
	}
}

// TestInspectFindsBoilerplateDirectory covers the template marker that is a
// directory rather than a file.
func TestInspectFindsBoilerplateDirectory(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	writeFileFS(t, fsys, "/component/.boilerplate/boilerplate.yml", "variables: []\n")

	markers, err := component.Inspect(fsys, "/component")
	require.NoError(t, err)
	assert.Equal(t, component.Markers{Template: true}, markers)
}

func TestKindConfigFile(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "terragrunt.hcl", component.KindUnit.ConfigFile())
	assert.Equal(t, "terragrunt.stack.hcl", component.KindStack.ConfigFile())
	assert.Empty(t, component.KindModule.ConfigFile())
	assert.Empty(t, component.KindTemplate.ConfigFile())
}
