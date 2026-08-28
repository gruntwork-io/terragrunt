package cli_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// listUnits is two units side by side, enough to show the column layout each
// format produces.
var listUnits = map[string]string{
	"a-unit/terragrunt.hcl": "",
	"b-unit/terragrunt.hcl": "",
}

func TestListCommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		want string
		args []string
	}{
		{
			name: "default format",
			want: "a-unit  b-unit  \n",
		},
		{
			name: "long format",
			args: []string{"--long"},
			want: `Type  Path
unit  a-unit
unit  b-unit
`,
		},
		{
			name: "tree format",
			args: []string{"--tree"},
			want: `.
├── a-unit
╰── b-unit
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, runList(t, listUnits, tc.args...))
		})
	}
}

func TestListCommandWithDependencies(t *testing.T) {
	t.Parallel()

	t.Run("tree nests each unit under what it depends on", func(t *testing.T) {
		t.Parallel()

		want := `.
├── stacks/live/dev
├── stacks/live/prod
├── units/live/dev/vpc
│   ├── units/live/dev/db
│   │   ╰── units/live/dev/ec2
│   ╰── units/live/dev/ec2
╰── units/live/prod/vpc
    ├── units/live/prod/db
    │   ╰── units/live/prod/ec2
    ╰── units/live/prod/ec2
`

		assert.Equal(t, want, runList(t, queueUnits, "--tree", "--dag"))
	})

	t.Run("long format lists them in a column", func(t *testing.T) {
		t.Parallel()

		want := `Type  Path                 Dependencies
stack stacks/live/dev
stack stacks/live/prod
unit  units/live/dev/db    units/live/dev/vpc
unit  units/live/dev/ec2   units/live/dev/db, units/live/dev/vpc
unit  units/live/dev/vpc
unit  units/live/prod/db   units/live/prod/vpc
unit  units/live/prod/ec2  units/live/prod/db, units/live/prod/vpc
unit  units/live/prod/vpc
`

		assert.Equal(t, want, runList(t, queueUnits, "--long", "--dependencies"))
	})
}

// TestListCommandWithExclude pins the exclude configs through every format,
// since dropping a unit has to hold for the columns and the tree alike.
func TestListCommandWithExclude(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		want string
		args []string
	}{
		{
			name: "plan drops the unit excluded from plan",
			args: []string{"--queue-construct-as", "plan"},
			want: "unit2  unit3  \n",
		},
		{
			name: "apply drops the unit excluded from apply",
			args: []string{"--queue-construct-as", "apply"},
			want: "unit1  unit3  \n",
		},
		{
			name: "long format",
			args: []string{"--queue-construct-as", "plan", "--long"},
			want: `Type  Path
unit  unit2
unit  unit3
`,
		},
		{
			name: "tree format",
			args: []string{"--queue-construct-as", "apply", "--tree"},
			want: `.
├── unit1
╰── unit3
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, runList(t, excludeUnits, tc.args...))
		})
	}
}

func TestListHidden(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"unit/terragrunt.hcl":        "",
		"stack/terragrunt.stack.hcl": "",
		".hide/unit/terragrunt.hcl":  "",
	}

	t.Run("hidden components are listed by default", func(t *testing.T) {
		t.Parallel()

		want := filepath.Join(".hide", "unit") + "  stack       unit        \n"
		assert.Equal(t, want, runList(t, files))
	})

	t.Run("no-hidden leaves them out", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "stack  unit   \n", runList(t, files, "--no-hidden"))
	})
}
